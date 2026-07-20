package drop

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Scheduler struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewScheduler(db *pgxpool.Pool, logger *slog.Logger) *Scheduler {
	return &Scheduler{db: db, logger: logger}
}

// Start claims expired reservations every 30 seconds. The claim and its
// outbox job are committed atomically, so any number of schedulers may run.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		s.logger.InfoContext(ctx, "scheduler started", slog.Duration("interval", 30*time.Second))
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

// processExpired changes pending reservations to the intermediate expiring
// state and persists one stable expiry job in the same transaction.
func (s *Scheduler) processExpired(ctx context.Context) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "expiry sweep transaction failed", slog.Any("err", err))
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, drop_id, user_id, size
		FROM reservations
		WHERE status = 'pending' AND expires_at < NOW()
		ORDER BY expires_at
		FOR UPDATE SKIP LOCKED
		LIMIT 500
	`)
	if err != nil {
		s.logger.ErrorContext(ctx, "expiry sweep failed", slog.Any("err", err))
		return
	}
	type expiredReservation struct {
		id, dropID, userID, size string
	}
	var reservations []expiredReservation
	for rows.Next() {
		var reservation expiredReservation
		if err := rows.Scan(&reservation.id, &reservation.dropID, &reservation.userID, &reservation.size); err != nil {
			rows.Close()
			s.logger.ErrorContext(ctx, "expiry row scan failed", slog.Any("err", err))
			return
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		s.logger.ErrorContext(ctx, "expiry rows failed", slog.Any("err", err))
		return
	}
	rows.Close()

	for _, reservation := range reservations {
		result, err := tx.Exec(ctx, `
			UPDATE reservations SET status = 'expiring'
			WHERE id = $1 AND status = 'pending'
		`, reservation.id)
		if err != nil {
			s.logger.ErrorContext(ctx, "reservation expiry claim failed", slog.String("reservation", reservation.id), slog.Any("err", err))
			return
		}
		if result.RowsAffected() != 1 {
			continue
		}
		payload := reservationExpiryPayload{
			ReservationID: reservation.id,
			DropID:        reservation.dropID,
			UserID:        reservation.userID,
			Size:          reservation.size,
		}
		if err := enqueueOutboxJob(ctx, tx, outboxJobReservationExpiry, reservationExpiryKey(reservation.id), payload); err != nil {
			s.logger.ErrorContext(ctx, "reservation expiry enqueue failed", slog.String("reservation", reservation.id), slog.Any("err", err))
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.logger.ErrorContext(ctx, "expiry sweep commit failed", slog.Any("err", err))
		return
	}
	if len(reservations) > 0 {
		s.logger.InfoContext(ctx, "expired reservations claimed", slog.Int("count", len(reservations)))
	}
}

func reservationExpiryKey(reservationID string) string {
	return fmt.Sprintf("reservation-expiry:%s", reservationID)
}
