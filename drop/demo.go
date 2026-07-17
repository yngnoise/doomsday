package drop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const DemoDropID = "demo-wraith-jacket"

var ErrDemoResetDisabled = errors.New("demo reset is disabled")

func DemoModeEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DEMO_MODE")), "true")
}

// ResetDemoData recreates the disposable dataset used by the public portfolio demo.
// It is deliberately unavailable unless DEMO_MODE=true because it clears the connected
// database and Redis logical database.
func ResetDemoData(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) error {
	if !DemoModeEnabled() {
		return ErrDemoResetDisabled
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin demo reset: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		TRUNCATE payment_events, payments, orders, reservations, otp_codes, users, drop_sizes, drops CASCADE
	`); err != nil {
		return fmt.Errorf("clear demo database: %w", err)
	}

	startsAt := time.Now().UTC().Add(-time.Minute)
	endsAt := startsAt.Add(24 * time.Hour)
	if _, err := tx.Exec(ctx, `
		INSERT INTO drops (id, name, description, price_cents, total_stock, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, DemoDropID, "WRAITH FIELD JACKET", "Portfolio demo drop. Simulated checkout only; no real charge is made.", 66600, 120, startsAt, endsAt); err != nil {
		return fmt.Errorf("seed demo drop: %w", err)
	}

	sizes := []string{"XS", "S", "M", "L", "XL", "XXL"}
	for _, size := range sizes {
		if _, err := tx.Exec(ctx, `INSERT INTO drop_sizes (drop_id, label, stock) VALUES ($1, $2, 20)`, DemoDropID, size); err != nil {
			return fmt.Errorf("seed demo size %s: %w", size, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo reset: %w", err)
	}

	if err := rdb.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("clear demo redis: %w", err)
	}
	pipe := rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("drop:%s:stock", DemoDropID), 120, 0)
	for _, size := range sizes {
		pipe.Set(ctx, fmt.Sprintf("drop:%s:size:%s:stock", DemoDropID, size), 20, 0)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("seed demo redis: %w", err)
	}
	return nil
}
