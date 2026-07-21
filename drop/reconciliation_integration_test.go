//go:build integration

package drop

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestReconciliationIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_ADDR are required")
	}
	t.Setenv("DEMO_MODE", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := ResetDemoData(ctx, db, rdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ResetDemoData(context.Background(), db, rdb) })

	reservationID := "reconcile-" + uuid.NewString()
	userID := "reconcile-user-" + uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at)
		VALUES ($1,$2,'load-item',$3,'M','expiring',NOW() + INTERVAL '10 minutes')
	`, reservationID, DemoDropID, userID); err != nil {
		t.Fatal(err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := NewHandler(ctx, rdb, db, NewHub(), NewMailer(logger), logger); err != nil {
		t.Fatalf("startup recovery failed: %v", err)
	}

	report, err := CheckDropInvariants(ctx, db, rdb, DemoDropID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.RedisStock != 119 || report.RedisReservationMarks != 1 {
		t.Fatalf("initial report = %#v", report)
	}

	if err := rdb.Decr(ctx, fmt.Sprintf("drop:%s:stock", DemoDropID)).Err(); err != nil {
		t.Fatal(err)
	}
	if err := rdb.Del(ctx, fmt.Sprintf("drop:%s:reservations", DemoDropID)).Err(); err != nil {
		t.Fatal(err)
	}
	report, err = CheckDropInvariants(ctx, db, rdb, DemoDropID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || len(report.Violations) < 2 {
		t.Fatalf("drift was not detected: %#v", report)
	}

	if err := ReconcileDropInventory(ctx, db, rdb, DemoDropID); err != nil {
		t.Fatal(err)
	}
	report, err = CheckDropInvariants(ctx, db, rdb, DemoDropID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("repair did not restore invariants: %#v", report)
	}
}
