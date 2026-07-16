//go:build integration

package drop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestReservationLifecycleIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_ADDR are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()
	concurrentDrop := "integration-concurrent-" + suffix
	expiryDrop := "integration-expiry-" + suffix
	checkoutDrop := "integration-checkout-" + suffix
	dropIDs := []string{concurrentDrop, expiryDrop, checkoutDrop}

	for _, dropID := range dropIDs {
		if _, err := db.Exec(ctx, `
			INSERT INTO drops (id, name, description, price_cents, total_stock, starts_at, ends_at)
			VALUES ($1, $2, '', 1500, 1, NOW() - INTERVAL '1 minute', NOW() + INTERVAL '1 hour')
		`, dropID, "Integration "+dropID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO drop_sizes (drop_id, label, stock) VALUES ($1, 'M', 1)
		`, dropID); err != nil {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		for _, dropID := range dropIDs {
			_, _ = db.Exec(context.Background(), `DELETE FROM orders WHERE drop_id = $1`, dropID)
			_, _ = db.Exec(context.Background(), `DELETE FROM reservations WHERE drop_id = $1`, dropID)
			_, _ = db.Exec(context.Background(), `DELETE FROM drop_sizes WHERE drop_id = $1`, dropID)
			_, _ = db.Exec(context.Background(), `DELETE FROM drops WHERE id = $1`, dropID)
			keys, _ := rdb.Keys(context.Background(), fmt.Sprintf("drop:%s:*", dropID)).Result()
			if len(keys) > 0 {
				_ = rdb.Del(context.Background(), keys...).Err()
			}
		}
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub()
	mailer := NewMailer(logger)
	handler, err := NewHandler(ctx, rdb, db, hub, mailer, logger)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("only one concurrent reservation wins the last item", func(t *testing.T) {
		users := []string{"user-a-" + suffix, "user-b-" + suffix}
		statuses := make(chan int, len(users))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, userID := range users {
			wg.Add(1)
			go func(userID string) {
				defer wg.Done()
				<-start
				body := bytes.NewBufferString(fmt.Sprintf(`{"drop_id":%q,"item_id":"item-1","size":"M"}`, concurrentDrop))
				request := httptest.NewRequest(http.MethodPost, "/api/reserve", body)
				request = request.WithContext(context.WithValue(request.Context(), CtxUserID, userID))
				response := httptest.NewRecorder()
				handler.ReserveItem(response, request)
				statuses <- response.Code
			}(userID)
		}
		close(start)
		wg.Wait()
		close(statuses)

		counts := map[int]int{}
		for status := range statuses {
			counts[status]++
		}
		if counts[http.StatusCreated] != 1 || counts[http.StatusGone] != 1 {
			t.Fatalf("reservation statuses = %#v", counts)
		}

		var reservations int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM reservations WHERE drop_id = $1`, concurrentDrop).Scan(&reservations); err != nil {
			t.Fatal(err)
		}
		if reservations != 1 {
			t.Fatalf("reservations = %d, want 1", reservations)
		}
	})

	t.Run("expiry restores stock once", func(t *testing.T) {
		reservationID := "expired-" + suffix
		userID := "expired-user-" + suffix
		if _, err := db.Exec(ctx, `
			INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at)
			VALUES ($1, $2, 'item-1', $3, 'M', 'pending', NOW() - INTERVAL '1 minute')
		`, reservationID, expiryDrop, userID); err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(ctx, fmt.Sprintf("drop:%s:stock", expiryDrop), 0, 0).Err(); err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(ctx, fmt.Sprintf("drop:%s:size:M:stock", expiryDrop), 0, 0).Err(); err != nil {
			t.Fatal(err)
		}
		if err := rdb.HSet(ctx, fmt.Sprintf("drop:%s:reservations", expiryDrop), userID, reservationID).Err(); err != nil {
			t.Fatal(err)
		}

		scheduler := NewScheduler(db, rdb, hub, mailer, logger)
		scheduler.processExpired(ctx)
		scheduler.processExpired(ctx)

		var status string
		if err := db.QueryRow(ctx, `SELECT status FROM reservations WHERE id = $1`, reservationID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		stock, err := rdb.Get(ctx, fmt.Sprintf("drop:%s:stock", expiryDrop)).Int64()
		if err != nil {
			t.Fatal(err)
		}
		if status != "expired" || stock != 1 {
			t.Fatalf("status = %q, stock = %d", status, stock)
		}
	})

	t.Run("concurrent checkout creates one order", func(t *testing.T) {
		reservationID := "checkout-" + suffix
		userID := "checkout-user-" + suffix
		email := "checkout@example.com"
		if _, err := db.Exec(ctx, `
			INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at)
			VALUES ($1, $2, 'item-1', $3, 'M', 'pending', NOW() + INTERVAL '10 minutes')
		`, reservationID, checkoutDrop, userID); err != nil {
			t.Fatal(err)
		}

		responses := make(chan map[string]string, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				request := httptest.NewRequest(http.MethodPost, "/api/checkout/"+reservationID+"/complete", bytes.NewBufferString(`{"name":"Test User","address":"Test Address"}`))
				request.SetPathValue("reservationID", reservationID)
				requestCtx := context.WithValue(request.Context(), CtxUserID, userID)
				requestCtx = context.WithValue(requestCtx, CtxEmail, email)
				response := httptest.NewRecorder()
				handler.CompleteCheckout(response, request.WithContext(requestCtx))
				if response.Code != http.StatusOK {
					responses <- map[string]string{"error": fmt.Sprintf("status %d: %s", response.Code, response.Body.String())}
					return
				}
				var payload map[string]string
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					responses <- map[string]string{"error": err.Error()}
					return
				}
				responses <- payload
			}()
		}
		close(start)
		wg.Wait()
		close(responses)

		var orderID string
		for response := range responses {
			if response["error"] != "" {
				t.Fatal(response["error"])
			}
			if orderID == "" {
				orderID = response["order_id"]
			} else if response["order_id"] != orderID {
				t.Fatalf("order IDs differ: %q and %q", orderID, response["order_id"])
			}
		}

		var orders int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE reservation_id = $1`, reservationID).Scan(&orders); err != nil {
			t.Fatal(err)
		}
		if orders != 1 {
			t.Fatalf("orders = %d, want 1", orders)
		}
	})
}
