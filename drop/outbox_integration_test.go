//go:build integration

package drop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recordingOutboxDispatcher struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (d *recordingOutboxDispatcher) Dispatch(context.Context, outboxJob) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return d.err
}

func (d *recordingOutboxDispatcher) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func TestOutboxWorkerIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	suffix := uuid.NewString()

	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM outbox_jobs WHERE idempotency_key LIKE $1`, "%"+suffix+"%")
	})

	t.Run("transaction rollback does not leave a job", func(t *testing.T) {
		key := "rollback-" + suffix
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := enqueueOutboxJob(ctx, tx, "test.rollback", key, map[string]string{"value": "discarded"}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_jobs WHERE idempotency_key=$1`, key).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rolled-back jobs = %d, want 0", count)
		}
	})

	t.Run("stable idempotency key deduplicates enqueue", func(t *testing.T) {
		key := "duplicate-" + suffix
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := enqueueOutboxJob(ctx, tx, "test.duplicate", key, map[string]string{"value": "same"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_jobs WHERE idempotency_key=$1`, key).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("jobs = %d, want 1", count)
		}
		if _, err := db.Exec(ctx, `DELETE FROM outbox_jobs WHERE idempotency_key=$1`, key); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("failed delivery retries then moves to dead letter", func(t *testing.T) {
		key := "retry-" + suffix
		if _, err := db.Exec(ctx, `
			INSERT INTO outbox_jobs (job_type, idempotency_key, payload, max_attempts)
			VALUES ('test.retry', $1, '{}', 2)
		`, key); err != nil {
			t.Fatal(err)
		}
		dispatcher := &recordingOutboxDispatcher{err: errors.New("temporary delivery failure")}
		worker := NewOutboxWorker(db, dispatcher, logger)
		worker.initialBackoff = time.Millisecond
		worker.maximumBackoff = time.Millisecond
		for attempt := 0; attempt < 2; attempt++ {
			if processed, err := worker.ProcessOne(ctx); err != nil || !processed {
				t.Fatalf("attempt %d: processed=%v err=%v", attempt+1, processed, err)
			}
			time.Sleep(3 * time.Millisecond)
		}
		var status string
		var attempts int
		var lastError string
		if err := db.QueryRow(ctx, `SELECT status, attempts, last_error FROM outbox_jobs WHERE idempotency_key=$1`, key).Scan(&status, &attempts, &lastError); err != nil {
			t.Fatal(err)
		}
		if status != "dead" || attempts != 2 || lastError == "" || dispatcher.callCount() != 2 {
			t.Fatalf("status=%q attempts=%d error=%q calls=%d", status, attempts, lastError, dispatcher.callCount())
		}
	})

	t.Run("stale processing lease is recovered after a crash", func(t *testing.T) {
		key := "crash-" + suffix
		if _, err := db.Exec(ctx, `
			INSERT INTO outbox_jobs (
				job_type, idempotency_key, payload, status, attempts,
				locked_at, locked_by, max_attempts
			) VALUES ('test.crash', $1, '{}', 'processing', 1, NOW() - INTERVAL '1 minute', 'dead-worker', 3)
		`, key); err != nil {
			t.Fatal(err)
		}
		dispatcher := &recordingOutboxDispatcher{}
		worker := NewOutboxWorker(db, dispatcher, logger)
		worker.leaseDuration = time.Millisecond
		if processed, err := worker.ProcessOne(ctx); err != nil || !processed {
			t.Fatalf("processed=%v err=%v", processed, err)
		}
		var status string
		var attempts int
		if err := db.QueryRow(ctx, `SELECT status, attempts FROM outbox_jobs WHERE idempotency_key=$1`, key).Scan(&status, &attempts); err != nil {
			t.Fatal(err)
		}
		if status != "completed" || attempts != 2 || dispatcher.callCount() != 1 {
			t.Fatalf("status=%q attempts=%d calls=%d", status, attempts, dispatcher.callCount())
		}
	})
}
