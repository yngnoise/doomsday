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
		UPDATE reservations
		SET status = 'expired'
		WHERE status = 'pending' AND expires_at < NOW()
		RETURNING drop_id
	`)
	if err != nil {
		s.logger.ErrorContext(ctx, "expiry sweep failed", slog.Any("err", err))
		return
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var dropID string
		if err := rows.Scan(&dropID); err == nil {
			counts[dropID]++
		}
	}

	for dropID, n := range counts {
		key := fmt.Sprintf("drop:%s:stock", dropID)
		newStock, err := s.redis.IncrBy(ctx, key, n).Result()
		if err != nil {
			s.logger.ErrorContext(ctx, "stock restore failed", slog.String("drop", dropID), slog.Any("err", err))
			continue
		}
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
		// In production userID would be an email or you'd look up the user record.
		// For now we log and send email if userID looks like an email.
		if len(userID) > 3 && containsAt(userID) {
			s.mailer.SendWaitlistPromotion(ctx, userID, dropID, dropName)
		}
	}
}

// containsAt is a minimal email check to avoid importing regexp.
func containsAt(s string) bool {
	for _, c := range s {
		if c == '@' {
			return true
		}
	}
	return false
}
