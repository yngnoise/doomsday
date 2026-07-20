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

type paymentSimulationPayload struct {
	PaymentID string `json:"payment_id"`
	Scenario  string `json:"scenario"`
}

type reservationEmailPayload struct {
	To            string    `json:"to"`
	Name          string    `json:"name"`
	ItemName      string    `json:"item_name"`
	ReservationID string    `json:"reservation_id"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type orderEmailPayload struct {
	To         string `json:"to"`
	Name       string `json:"name"`
	ItemName   string `json:"item_name"`
	OrderID    string `json:"order_id"`
	PriceCents int    `json:"price_cents"`
}

type reservationExpiryPayload struct {
	ReservationID string `json:"reservation_id"`
	DropID        string `json:"drop_id"`
	UserID        string `json:"user_id"`
	Size          string `json:"size"`
}

type waitlistPromotionPayload struct {
	ReservationID string `json:"reservation_id"`
	DropID        string `json:"drop_id"`
}

type applicationJobDispatcher struct {
	db                    *pgxpool.Pool
	redis                 *redis.Client
	hub                   *Hub
	mailer                *Mailer
	logger                *slog.Logger
	processPaymentWebhook func(context.Context, []byte, string) error
	paymentWebhookSecret  string
}

func NewApplicationJobDispatcher(
	db *pgxpool.Pool,
	rdb *redis.Client,
	hub *Hub,
	mailer *Mailer,
	logger *slog.Logger,
	handler *Handler,
) *applicationJobDispatcher {
	return &applicationJobDispatcher{
		db: db, redis: rdb, hub: hub, mailer: mailer, logger: logger,
		processPaymentWebhook: handler.processPaymentWebhook, paymentWebhookSecret: handler.paymentWebhookSecret,
	}
}

func (d *applicationJobDispatcher) Dispatch(ctx context.Context, job outboxJob) error {
	switch job.Type {
	case outboxJobPaymentSimulation:
		var payload paymentSimulationPayload
		if err := decodeOutboxPayload(job, &payload); err != nil {
			return err
		}
		return d.simulatePayment(ctx, payload)
	case outboxJobReservationEmail:
		var payload reservationEmailPayload
		if err := decodeOutboxPayload(job, &payload); err != nil {
			return err
		}
		if !d.mailer.Enabled() {
			d.logger.InfoContext(ctx, "reservation email skipped (SMTP not configured)", slog.String("job_id", job.ID))
			return nil
		}
		return d.mailer.SendReservationConfirmation(ctx, payload.To, payload.Name, payload.ItemName, payload.ReservationID, payload.ExpiresAt, job.IdempotencyKey)
	case outboxJobOrderEmail:
		var payload orderEmailPayload
		if err := decodeOutboxPayload(job, &payload); err != nil {
			return err
		}
		if !d.mailer.Enabled() {
			d.logger.InfoContext(ctx, "order email skipped (SMTP not configured)", slog.String("job_id", job.ID))
			return nil
		}
		return d.mailer.SendOrderConfirmation(ctx, payload.To, payload.Name, payload.ItemName, payload.OrderID, payload.PriceCents, job.IdempotencyKey)
	case outboxJobReservationExpiry:
		var payload reservationExpiryPayload
		if err := decodeOutboxPayload(job, &payload); err != nil {
			return err
		}
		return d.expireReservation(ctx, payload)
	case outboxJobWaitlistPromotion:
		var payload waitlistPromotionPayload
		if err := decodeOutboxPayload(job, &payload); err != nil {
			return err
		}
		return d.promoteWaitlist(ctx, job.IdempotencyKey, payload)
	default:
		return fmt.Errorf("unsupported outbox job type %q", job.Type)
	}
}

func decodeOutboxPayload(job outboxJob, target any) error {
	if err := json.Unmarshal(job.Payload, target); err != nil {
		return fmt.Errorf("decode %s payload: %w", job.Type, err)
	}
	return nil
}

func (d *applicationJobDispatcher) simulatePayment(ctx context.Context, payload paymentSimulationPayload) error {
	if err := d.deliverStablePaymentEvent(ctx, payload.PaymentID, "payment.processing"); err != nil {
		return err
	}
	delay := 450 * time.Millisecond
	eventType := "payment.succeeded"
	switch payload.Scenario {
	case "declined":
		eventType = "payment.declined"
	case "cancelled":
		eventType = "payment.cancelled"
	case "timeout":
		eventType = "payment.timed_out"
		delay = 1500 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return d.deliverStablePaymentEvent(ctx, payload.PaymentID, eventType)
}

func (d *applicationJobDispatcher) deliverStablePaymentEvent(ctx context.Context, paymentID, eventType string) error {
	event := paymentEvent{
		ID:         "EVT-" + paymentID + "-" + eventType,
		PaymentID:  paymentID,
		Type:       eventType,
		OccurredAt: time.Now().UTC(),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return d.processPaymentWebhook(ctx, body, signPaymentPayload(d.paymentWebhookSecret, body))
}

func (d *applicationJobDispatcher) expireReservation(ctx context.Context, payload reservationExpiryPayload) error {
	release, err := releaseReservationInRedis(ctx, d.redis, payload.DropID, payload.Size, payload.UserID, payload.ReservationID)
	if err != nil {
		return err
	}

	tx, err := d.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE reservations SET status = 'expired'
		WHERE id = $1 AND status = 'expiring'
	`, payload.ReservationID); err != nil {
		return err
	}
	if err := enqueueOutboxJob(ctx, tx, outboxJobWaitlistPromotion,
		"waitlist-promotion:"+payload.ReservationID,
		waitlistPromotionPayload{ReservationID: payload.ReservationID, DropID: payload.DropID},
	); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if release.Released {
		d.hub.Broadcast(payload.DropID, release.TotalStock)
	}
	return nil
}

var claimWaitlistPromotionScript = redis.NewScript(`
local queue_key = KEYS[1]
local claims_key = KEYS[2]
local job_key = ARGV[1]
local none = '__none__'

local existing = redis.call('HGET', claims_key, job_key)
if existing then return existing end

local popped = redis.call('ZPOPMIN', queue_key, 1)
if #popped == 0 then
  redis.call('HSET', claims_key, job_key, none)
  return none
end

redis.call('HSET', claims_key, job_key, popped[1])
return popped[1]
`)

func (d *applicationJobDispatcher) promoteWaitlist(ctx context.Context, idempotencyKey string, payload waitlistPromotionPayload) error {
	queueKey := fmt.Sprintf("drop:%s:waitlist", payload.DropID)
	claimsKey := fmt.Sprintf("drop:%s:waitlist-promotions", payload.DropID)
	userID, err := claimWaitlistPromotionScript.Run(ctx, d.redis, []string{queueKey, claimsKey}, idempotencyKey).Text()
	if err != nil {
		return fmt.Errorf("claim waitlist promotion: %w", err)
	}
	if userID == "__none__" {
		return nil
	}

	var email, dropName string
	if err := d.db.QueryRow(ctx, `
		SELECT u.email, d.name FROM users u CROSS JOIN drops d
		WHERE u.id = $1 AND d.id = $2
	`, userID, payload.DropID).Scan(&email, &dropName); err != nil {
		return fmt.Errorf("load waitlist recipient: %w", err)
	}
	if !d.mailer.Enabled() {
		d.logger.InfoContext(ctx, "waitlist email skipped (SMTP not configured)", slog.String("job_id", idempotencyKey))
		return nil
	}
	return d.mailer.SendWaitlistPromotion(ctx, email, payload.DropID, dropName, idempotencyKey)
}
