package drop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	outboxJobPaymentSimulation  = "payment.simulation"
	outboxJobReservationEmail   = "email.reservation_confirmation"
	outboxJobOrderEmail         = "email.order_confirmation"
	outboxJobReservationExpiry  = "reservation.expiry"
	outboxJobWaitlistPromotion  = "waitlist.promotion"
	outboxDefaultMaxAttempts    = 8
	outboxDefaultPollInterval   = 250 * time.Millisecond
	outboxDefaultLeaseDuration  = 30 * time.Second
	outboxDefaultInitialBackoff = time.Second
	outboxDefaultMaximumBackoff = 5 * time.Minute
)

type outboxJob struct {
	ID             string
	Type           string
	IdempotencyKey string
	Payload        json.RawMessage
	Attempts       int
	MaxAttempts    int
	CorrelationID  string
	TraceParent    string
	TraceState     string
}

func enqueueOutboxJob(ctx context.Context, executor pgx.Tx, jobType, idempotencyKey string, payload any) error {
	ctx, correlationID := ensureCorrelationID(ctx)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO outbox_jobs (
			job_type, idempotency_key, payload, max_attempts,
			correlation_id, trace_parent, trace_state
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, jobType, idempotencyKey, encoded, outboxDefaultMaxAttempts,
		correlationID, carrier.Get("traceparent"), carrier.Get("tracestate"),
	)
	if err != nil {
		return fmt.Errorf("enqueue outbox job %q: %w", idempotencyKey, err)
	}
	return nil
}

type outboxDispatcher interface {
	Dispatch(context.Context, outboxJob) error
}

type OutboxWorker struct {
	db             *pgxpool.Pool
	dispatcher     outboxDispatcher
	logger         *slog.Logger
	workerID       string
	pollInterval   time.Duration
	leaseDuration  time.Duration
	initialBackoff time.Duration
	maximumBackoff time.Duration
	telemetry      *Telemetry
	wg             sync.WaitGroup
}

func NewOutboxWorker(db *pgxpool.Pool, dispatcher outboxDispatcher, logger *slog.Logger, telemetry ...*Telemetry) *OutboxWorker {
	worker := &OutboxWorker{
		db:             db,
		dispatcher:     dispatcher,
		logger:         logger,
		workerID:       "worker-" + uuid.NewString(),
		pollInterval:   outboxDefaultPollInterval,
		leaseDuration:  outboxDefaultLeaseDuration,
		initialBackoff: outboxDefaultInitialBackoff,
		maximumBackoff: outboxDefaultMaximumBackoff,
	}
	if len(telemetry) > 0 {
		worker.telemetry = telemetry[0]
	}
	return worker
}

func (w *OutboxWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		w.logger.InfoContext(ctx, "outbox worker started", slog.String("worker_id", w.workerID))
		for {
			processed, err := w.ProcessOne(ctx)
			if err != nil && ctx.Err() == nil {
				w.logger.ErrorContext(ctx, "outbox worker iteration failed", slog.Any("err", err))
			}
			if processed {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (w *OutboxWorker) Wait() {
	w.wg.Wait()
}

func (w *OutboxWorker) ProcessOne(ctx context.Context) (bool, error) {
	job, found, err := w.claim(ctx)
	if err != nil || !found {
		return false, err
	}

	dispatchCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{
		"traceparent": job.TraceParent,
		"tracestate":  job.TraceState,
	})
	dispatchCtx = ContextWithCorrelationID(dispatchCtx, job.CorrelationID)
	dispatchCtx, span := otel.Tracer(instrumentationName).Start(
		dispatchCtx,
		"outbox "+job.Type,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.operation.type", "process"),
			attribute.String("messaging.destination.name", job.Type),
			attribute.Int("messaging.message.delivery_count", job.Attempts),
		),
	)
	defer span.End()
	if job.Attempts > 1 {
		w.telemetry.RecordOutbox(job.Type, "recovered")
	}

	if err := w.dispatcher.Dispatch(dispatchCtx, job); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "job dispatch failed")
		if markErr := w.markFailed(dispatchCtx, job, err); markErr != nil {
			return true, fmt.Errorf("dispatch %s: %v; record failure: %w", job.ID, err, markErr)
		}
		return true, nil
	}
	if err := w.markCompleted(dispatchCtx, job.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "job completion failed")
		return true, err
	}
	w.telemetry.RecordOutbox(job.Type, "completed")
	return true, nil
}

func (w *OutboxWorker) claim(ctx context.Context) (outboxJob, bool, error) {
	var job outboxJob
	err := w.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM outbox_jobs
			WHERE (status = 'pending' AND available_at <= NOW())
			   OR (status = 'processing' AND locked_at < NOW() - ($2 * INTERVAL '1 millisecond'))
			ORDER BY available_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE outbox_jobs AS job
		SET status = 'processing', attempts = job.attempts + 1,
			locked_at = NOW(), locked_by = $1, updated_at = NOW()
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.id, job.job_type, job.idempotency_key, job.payload,
			job.attempts, job.max_attempts, job.correlation_id,
			job.trace_parent, job.trace_state
	`, w.workerID, w.leaseDuration.Milliseconds()).Scan(
		&job.ID, &job.Type, &job.IdempotencyKey, &job.Payload, &job.Attempts, &job.MaxAttempts,
		&job.CorrelationID, &job.TraceParent, &job.TraceState,
	)
	if err == pgx.ErrNoRows {
		return outboxJob{}, false, nil
	}
	if err != nil {
		return outboxJob{}, false, fmt.Errorf("claim outbox job: %w", err)
	}
	return job, true, nil
}

func (w *OutboxWorker) markCompleted(ctx context.Context, jobID string) error {
	result, err := w.db.Exec(ctx, `
		UPDATE outbox_jobs
		SET status = 'completed', completed_at = NOW(), locked_at = NULL,
			locked_by = NULL, last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND locked_by = $2
	`, jobID, w.workerID)
	if err != nil {
		return fmt.Errorf("complete outbox job %s: %w", jobID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("complete outbox job %s: lease lost", jobID)
	}
	return nil
}

func (w *OutboxWorker) markFailed(ctx context.Context, job outboxJob, dispatchErr error) error {
	status := "pending"
	availableAt := time.Now().UTC().Add(outboxBackoff(job.Attempts, w.initialBackoff, w.maximumBackoff))
	if job.Attempts >= job.MaxAttempts {
		status = "dead"
		availableAt = time.Now().UTC()
	}
	metricOutcome := "retry"
	if status == "dead" {
		metricOutcome = "dead"
	}
	w.telemetry.RecordOutbox(job.Type, metricOutcome)
	result, err := w.db.Exec(ctx, `
		UPDATE outbox_jobs
		SET status = $3, available_at = $4, locked_at = NULL, locked_by = NULL,
			last_error = $5, updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND locked_by = $2
	`, job.ID, w.workerID, status, availableAt, truncateOutboxError(dispatchErr),
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("lease lost")
	}
	w.logger.WarnContext(ctx, "outbox job failed",
		slog.String("job_id", job.ID),
		slog.String("job_type", job.Type),
		slog.Int("attempt", job.Attempts),
		slog.String("status", status),
		slog.Any("err", dispatchErr),
	)
	return nil
}

func outboxBackoff(attempt int, initial, maximum time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := initial
	for i := 1; i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func truncateOutboxError(err error) string {
	const maxLength = 2000
	message := err.Error()
	if len(message) > maxLength {
		return message[:maxLength]
	}
	return message
}
