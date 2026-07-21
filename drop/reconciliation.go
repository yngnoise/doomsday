package drop

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type SizeInvariant struct {
	Label         string `json:"label"`
	BaselineStock int64  `json:"baseline_stock"`
	Consumed      int64  `json:"consumed"`
	ExpectedStock int64  `json:"expected_stock"`
	RedisStock    int64  `json:"redis_stock"`
}

type DropInvariantReport struct {
	DropID                string          `json:"drop_id"`
	Valid                 bool            `json:"valid"`
	BaselineStock         int64           `json:"baseline_stock"`
	ConsumedReservations  int64           `json:"consumed_reservations"`
	ExpectedStock         int64           `json:"expected_stock"`
	RedisStock            int64           `json:"redis_stock"`
	RedisReservationMarks int64           `json:"redis_reservation_markers"`
	CompletedReservations int64           `json:"completed_reservations"`
	Orders                int64           `json:"orders"`
	SettledPayments       int64           `json:"settled_payments"`
	Sizes                 []SizeInvariant `json:"sizes"`
	Violations            []string        `json:"violations"`
}

type inventorySnapshot struct {
	baseline     int64
	consumed     int64
	sizeConsumed int64
	sizes        []SizeInvariant
	markers      map[string]string
}

// CheckDropInvariants compares PostgreSQL's durable state with Redis's live
// counters and reservation markers. Reports contain counts only, never user or
// reservation identifiers.
func CheckDropInvariants(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, dropID string) (DropInvariantReport, error) {
	snapshot, err := loadInventorySnapshot(ctx, db, dropID)
	if err != nil {
		return DropInvariantReport{}, err
	}
	report := DropInvariantReport{
		DropID: dropID, BaselineStock: snapshot.baseline, ConsumedReservations: snapshot.consumed,
		ExpectedStock: snapshot.baseline - snapshot.consumed, Sizes: snapshot.sizes, Violations: []string{},
	}

	report.RedisStock, err = redisCounter(ctx, rdb, fmt.Sprintf("drop:%s:stock", dropID))
	if errors.Is(err, redis.Nil) {
		report.Violations = append(report.Violations, "Redis total stock counter is missing")
		report.RedisStock = -1
	} else if err != nil {
		return DropInvariantReport{}, fmt.Errorf("read Redis total stock: %w", err)
	}
	if report.RedisStock != report.ExpectedStock {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"total stock mismatch: expected %d, Redis has %d", report.ExpectedStock, report.RedisStock,
		))
	}
	if report.RedisStock < 0 {
		report.Violations = append(report.Violations, "Redis total stock is negative")
	}

	var redisSizeTotal, baselineSizeTotal int64
	for index := range report.Sizes {
		size := &report.Sizes[index]
		baselineSizeTotal += size.BaselineStock
		if size.ExpectedStock < 0 {
			report.Violations = append(report.Violations, fmt.Sprintf("size %s is oversubscribed in PostgreSQL", size.Label))
		}
		size.RedisStock, err = redisCounter(ctx, rdb, fmt.Sprintf("drop:%s:size:%s:stock", dropID, size.Label))
		if errors.Is(err, redis.Nil) {
			report.Violations = append(report.Violations, fmt.Sprintf("Redis size counter %s is missing", size.Label))
			size.RedisStock = -1
		} else if err != nil {
			return DropInvariantReport{}, fmt.Errorf("read Redis size %s stock: %w", size.Label, err)
		}
		if size.RedisStock != size.ExpectedStock {
			report.Violations = append(report.Violations, fmt.Sprintf(
				"size %s stock mismatch: expected %d, Redis has %d", size.Label, size.ExpectedStock, size.RedisStock,
			))
		}
		if size.RedisStock < 0 {
			report.Violations = append(report.Violations, fmt.Sprintf("Redis size %s stock is negative", size.Label))
		}
		redisSizeTotal += size.RedisStock
	}
	if baselineSizeTotal != report.BaselineStock {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"PostgreSQL size baselines sum to %d but drop baseline is %d", baselineSizeTotal, report.BaselineStock,
		))
	}
	if snapshot.sizeConsumed != snapshot.consumed {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"active reservations=%d but reservations assigned to configured sizes=%d",
			snapshot.consumed, snapshot.sizeConsumed,
		))
	}
	if redisSizeTotal != report.RedisStock {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"Redis size counters sum to %d but total counter is %d", redisSizeTotal, report.RedisStock,
		))
	}

	redisMarkers, err := rdb.HGetAll(ctx, fmt.Sprintf("drop:%s:reservations", dropID)).Result()
	if err != nil {
		return DropInvariantReport{}, fmt.Errorf("read Redis reservation markers: %w", err)
	}
	report.RedisReservationMarks = int64(len(redisMarkers))
	if len(redisMarkers) != len(snapshot.markers) {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"reservation marker count mismatch: PostgreSQL has %d, Redis has %d", len(snapshot.markers), len(redisMarkers),
		))
	}
	mismatchedMarkers := 0
	for userID, reservationID := range snapshot.markers {
		if redisMarkers[userID] != reservationID {
			mismatchedMarkers++
		}
	}
	if mismatchedMarkers > 0 {
		report.Violations = append(report.Violations, fmt.Sprintf("%d reservation markers do not match durable state", mismatchedMarkers))
	}

	if err := db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM reservations WHERE drop_id=$1 AND status='completed'),
			(SELECT COUNT(*) FROM orders WHERE drop_id=$1),
			(SELECT COUNT(*) FROM payments p JOIN reservations r ON r.id=p.reservation_id
			 WHERE r.drop_id=$1 AND p.status IN ('paid','refunded'))
	`, dropID).Scan(
		&report.CompletedReservations, &report.Orders, &report.SettledPayments,
	); err != nil {
		return DropInvariantReport{}, fmt.Errorf("query order invariants: %w", err)
	}

	var ordersWithoutPayment, paymentsWithoutOrder int64
	if err := db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM orders o LEFT JOIN payments p ON p.id=o.payment_id
			 WHERE o.drop_id=$1 AND (p.id IS NULL OR p.status NOT IN ('paid','refunded'))),
			(SELECT COUNT(*) FROM payments p JOIN reservations r ON r.id=p.reservation_id
			 LEFT JOIN orders o ON o.payment_id=p.id
			 WHERE r.drop_id=$1 AND p.status IN ('paid','refunded') AND o.id IS NULL)
	`, dropID).Scan(&ordersWithoutPayment, &paymentsWithoutOrder); err != nil {
		return DropInvariantReport{}, fmt.Errorf("query payment/order links: %w", err)
	}
	if report.CompletedReservations != report.Orders {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"completed reservations=%d but orders=%d", report.CompletedReservations, report.Orders,
		))
	}
	if report.Orders != report.SettledPayments {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"orders=%d but paid/refunded payments=%d", report.Orders, report.SettledPayments,
		))
	}
	if ordersWithoutPayment != 0 || paymentsWithoutOrder != 0 {
		report.Violations = append(report.Violations, fmt.Sprintf(
			"invalid payment/order links: orders_without_settled_payment=%d settled_payments_without_order=%d",
			ordersWithoutPayment, paymentsWithoutOrder,
		))
	}

	report.Valid = len(report.Violations) == 0
	return report, nil
}

// ReconcileDropInventory rebuilds Redis from PostgreSQL. Callers must quiesce
// writes for the target drop while it runs.
func ReconcileDropInventory(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, dropID string) error {
	snapshot, err := loadInventorySnapshot(ctx, db, dropID)
	if err != nil {
		return err
	}
	_, err = rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, fmt.Sprintf("drop:%s:stock", dropID), snapshot.baseline-snapshot.consumed, 0)
		pipe.Del(ctx, fmt.Sprintf("drop:%s:meta", dropID))
		markerKey := fmt.Sprintf("drop:%s:reservations", dropID)
		pipe.Del(ctx, markerKey)
		for _, size := range snapshot.sizes {
			pipe.Set(ctx, fmt.Sprintf("drop:%s:size:%s:stock", dropID, size.Label), size.ExpectedStock, 0)
		}
		if len(snapshot.markers) > 0 {
			values := make([]any, 0, len(snapshot.markers)*2)
			users := make([]string, 0, len(snapshot.markers))
			for userID := range snapshot.markers {
				users = append(users, userID)
			}
			sort.Strings(users)
			for _, userID := range users {
				values = append(values, userID, snapshot.markers[userID])
			}
			pipe.HSet(ctx, markerKey, values...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rebuild Redis inventory: %w", err)
	}
	return nil
}

func loadInventorySnapshot(ctx context.Context, db *pgxpool.Pool, dropID string) (inventorySnapshot, error) {
	var snapshot inventorySnapshot
	snapshot.markers = map[string]string{}
	if err := db.QueryRow(ctx, `SELECT total_stock FROM drops WHERE id=$1`, dropID).Scan(&snapshot.baseline); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return inventorySnapshot{}, fmt.Errorf("drop %q does not exist", dropID)
		}
		return inventorySnapshot{}, fmt.Errorf("load drop inventory: %w", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM reservations
		WHERE drop_id=$1 AND status IN ('pending','expiring','completed')
	`, dropID).Scan(&snapshot.consumed); err != nil {
		return inventorySnapshot{}, fmt.Errorf("count consumed reservations: %w", err)
	}
	rows, err := db.Query(ctx, `
		SELECT ds.label, ds.stock,
		       COUNT(r.id) FILTER (WHERE r.status IN ('pending','expiring','completed'))
		FROM drop_sizes ds
		LEFT JOIN reservations r ON r.drop_id=ds.drop_id AND r.size=ds.label
		WHERE ds.drop_id=$1
		GROUP BY ds.label, ds.stock
		ORDER BY ds.label
	`, dropID)
	if err != nil {
		return inventorySnapshot{}, fmt.Errorf("load size inventory: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var size SizeInvariant
		if err := rows.Scan(&size.Label, &size.BaselineStock, &size.Consumed); err != nil {
			return inventorySnapshot{}, err
		}
		size.ExpectedStock = size.BaselineStock - size.Consumed
		snapshot.sizes = append(snapshot.sizes, size)
		snapshot.sizeConsumed += size.Consumed
	}
	if err := rows.Err(); err != nil {
		return inventorySnapshot{}, err
	}

	markerRows, err := db.Query(ctx, `
		SELECT user_id, id FROM reservations
		WHERE drop_id=$1 AND status IN ('pending','expiring','completed')
	`, dropID)
	if err != nil {
		return inventorySnapshot{}, fmt.Errorf("load reservation markers: %w", err)
	}
	defer markerRows.Close()
	for markerRows.Next() {
		var userID, reservationID string
		if err := markerRows.Scan(&userID, &reservationID); err != nil {
			return inventorySnapshot{}, err
		}
		if _, duplicate := snapshot.markers[userID]; duplicate {
			return inventorySnapshot{}, fmt.Errorf("multiple active reservations exist for one user in drop %q", dropID)
		}
		snapshot.markers[userID] = reservationID
	}
	if err := markerRows.Err(); err != nil {
		return inventorySnapshot{}, err
	}
	return snapshot, nil
}

func redisCounter(ctx context.Context, rdb *redis.Client, key string) (int64, error) {
	return rdb.Get(ctx, key).Int64()
}
