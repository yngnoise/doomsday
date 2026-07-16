package drop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Scheduler struct {
	db     *pgxpool.Pool
	redis  *redis.Client
	hub    *Hub
	mailer *Mailer
	logger *slog.Logger
}

func NewScheduler(db *pgxpool.Pool, rdb *redis.Client, hub *Hub, mailer *Mailer, logger *slog.Logger) *Scheduler {
	return &Scheduler{db: db, redis: rdb, hub: hub, mailer: mailer, logger: logger}
}

// Start launches the background expiry sweep every 30 seconds.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		s.logger.InfoContext(ctx, "scheduler started — expiry sweep every 30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processExpired(ctx)
			}
		}
	}()
}

// processExpired marks timed-out pending reservations as expired,
// restores their stock in Redis, broadcasts via SSE, and promotes the waitlist.
func (s *Scheduler) processExpired(ctx context.Context) {
	rows, err := s.db.Query(ctx, `
		SELECT id, drop_id, user_id, size
		FROM reservations
		WHERE status = 'pending' AND expires_at < NOW()
		ORDER BY expires_at
		LIMIT 500
	`)
	if err != nil {
		s.logger.ErrorContext(ctx, "expiry sweep failed", slog.Any("err", err))
		return
	}
	type expiredReservation struct {
		id, dropID, userID, size string
	}
	var expired []expiredReservation
	for rows.Next() {
		var reservation expiredReservation
		if err := rows.Scan(&reservation.id, &reservation.dropID, &reservation.userID, &reservation.size); err != nil {
			s.logger.ErrorContext(ctx, "expiry row scan failed", slog.Any("err", err))
			continue
		}
		expired = append(expired, reservation)
	}
	if err := rows.Err(); err != nil {
		s.logger.ErrorContext(ctx, "expiry rows failed", slog.Any("err", err))
	}
	rows.Close()

	counts := make(map[string]int64)
	latestStock := make(map[string]int64)
	for _, reservation := range expired {
		release, err := releaseReservationInRedis(
			ctx,
			s.redis,
			reservation.dropID,
			reservation.size,
			reservation.userID,
			reservation.id,
		)
		if err != nil {
			s.logger.ErrorContext(ctx, "reservation release failed",
				slog.String("reservation", reservation.id),
				slog.String("drop", reservation.dropID),
				slog.Any("err", err),
			)
			continue
		}

		if _, err := s.db.Exec(ctx, `
			UPDATE reservations
			SET status = 'expired'
			WHERE id = $1 AND status = 'pending'
		`, reservation.id); err != nil {
			s.logger.ErrorContext(ctx, "reservation expiry status update failed",
				slog.String("reservation", reservation.id),
				slog.Any("err", err),
			)
		}

		if release.Released {
			counts[reservation.dropID]++
			latestStock[reservation.dropID] = release.TotalStock
		}
	}

	for dropID, n := range counts {
		newStock := latestStock[dropID]
		s.hub.Broadcast(dropID, newStock)
		s.logger.InfoContext(ctx, "stock restored",
			slog.String("drop", dropID),
			slog.Int64("restored", n),
			slog.Int64("new_stock", newStock),
		)
		s.promoteWaitlist(ctx, dropID, n)
	}
}

// promoteWaitlist pops the next N users from the sorted-set queue
// and sends each a promotion email.
func (s *Scheduler) promoteWaitlist(ctx context.Context, dropID string, n int64) {
	key := fmt.Sprintf("drop:%s:waitlist", dropID)
	promoted, err := s.redis.ZPopMin(ctx, key, n).Result()
	if err != nil || len(promoted) == 0 {
		return
	}

	// Fetch drop name for the email
	dropName := dropID
	if raw, err := s.redis.Get(ctx, fmt.Sprintf("drop:%s:meta", dropID)).Bytes(); err == nil {
		var d struct {
			Name string `json:"name"`
		}
		if jsonErr := json.Unmarshal(raw, &d); jsonErr == nil && d.Name != "" {
			dropName = d.Name
		}
	}

	for _, m := range promoted {
		userID, _ := m.Member.(string)
		s.logger.InfoContext(ctx, "waitlist user promoted",
			slog.String("drop", dropID),
			slog.String("user", userID),
		)
		var email string
		if err := s.db.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
			s.logger.ErrorContext(ctx, "waitlist user lookup failed",
				slog.String("drop", dropID),
				slog.String("user", userID),
				slog.Any("err", err),
			)
			if addErr := s.redis.ZAdd(ctx, key, redis.Z{Score: m.Score, Member: userID}).Err(); addErr != nil {
				s.logger.ErrorContext(ctx, "waitlist requeue failed",
					slog.String("drop", dropID),
					slog.String("user", userID),
					slog.Any("err", addErr),
				)
			}
			continue
		}
		if err := s.mailer.SendWaitlistPromotion(ctx, email, dropID, dropName); err != nil {
			if addErr := s.redis.ZAdd(ctx, key, redis.Z{Score: m.Score, Member: userID}).Err(); addErr != nil {
				s.logger.ErrorContext(ctx, "waitlist requeue after email failure failed",
					slog.String("drop", dropID),
					slog.String("user", userID),
					slog.Any("err", addErr),
				)
			}
		}
	}
}
