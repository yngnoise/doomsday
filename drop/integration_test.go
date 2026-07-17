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

func TestDemoResetIntegration(t *testing.T) {
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

	for attempt := 0; attempt < 2; attempt++ {
		if err := ResetDemoData(ctx, db, rdb); err != nil {
			t.Fatalf("ResetDemoData() attempt %d error = %v", attempt+1, err)
		}
	}

	var dropCount, stock int
	if err := db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(MAX(total_stock), 0) FROM drops WHERE id=$1`, DemoDropID).Scan(&dropCount, &stock); err != nil {
		t.Fatal(err)
	}
	if dropCount != 1 || stock != 120 {
		t.Fatalf("demo drop count=%d stock=%d, want 1 and 120", dropCount, stock)
	}
	redisStock, err := rdb.Get(ctx, fmt.Sprintf("drop:%s:stock", DemoDropID)).Int()
	if err != nil || redisStock != 120 {
		t.Fatalf("demo Redis stock=%d err=%v, want 120", redisStock, err)
	}
}

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
	failureDrop := "integration-payment-retry-" + suffix
	dropIDs := []string{concurrentDrop, expiryDrop, checkoutDrop, failureDrop}

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
			_, _ = db.Exec(context.Background(), `DELETE FROM payment_events WHERE payment_id IN (SELECT p.id FROM payments p JOIN reservations r ON r.id=p.reservation_id WHERE r.drop_id=$1)`, dropID)
			_, _ = db.Exec(context.Background(), `DELETE FROM orders WHERE drop_id = $1`, dropID)
			_, _ = db.Exec(context.Background(), `DELETE FROM payments WHERE reservation_id IN (SELECT id FROM reservations WHERE drop_id=$1)`, dropID)
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

	t.Run("concurrent payment creation and duplicate webhook create one order", func(t *testing.T) {
		reservationID := "checkout-" + suffix
		userID := "checkout-user-" + suffix
		email := "checkout@example.com"
		if _, err := db.Exec(ctx, `
			INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at)
			VALUES ($1, $2, 'item-1', $3, 'M', 'pending', NOW() + INTERVAL '10 minutes')
		`, reservationID, checkoutDrop, userID); err != nil {
			t.Fatal(err)
		}

		responses := make(chan paymentResponse, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				request := httptest.NewRequest(http.MethodPost, "/api/checkout/"+reservationID+"/payments", bytes.NewBufferString(`{"name":"Test User","address":"Test Address","scenario":"success"}`))
				request.SetPathValue("reservationID", reservationID)
				requestCtx := context.WithValue(request.Context(), CtxUserID, userID)
				requestCtx = context.WithValue(requestCtx, CtxEmail, email)
				response := httptest.NewRecorder()
				handler.CreatePayment(response, request.WithContext(requestCtx))
				if response.Code != http.StatusOK && response.Code != http.StatusAccepted {
					t.Errorf("status %d: %s", response.Code, response.Body.String())
					return
				}
				var payload paymentResponse
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Error(err)
					return
				}
				responses <- payload
			}()
		}
		close(start)
		wg.Wait()
		close(responses)

		var paymentID string
		for response := range responses {
			if paymentID == "" {
				paymentID = response.PaymentID
			} else if response.PaymentID != paymentID {
				t.Fatalf("payment IDs differ: %q and %q", paymentID, response.PaymentID)
			}
		}
		if paymentID == "" {
			t.Fatal("payment was not created")
		}

		deadline := time.Now().Add(4 * time.Second)
		var paymentStatus string
		for time.Now().Before(deadline) {
			if err := db.QueryRow(ctx, `SELECT status FROM payments WHERE id=$1`, paymentID).Scan(&paymentStatus); err != nil {
				t.Fatal(err)
			}
			if paymentStatus == "paid" {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if paymentStatus != "paid" {
			t.Fatalf("payment status = %q, want paid", paymentStatus)
		}

		event := paymentEvent{ID: "duplicate-" + suffix, PaymentID: paymentID, Type: "payment.succeeded", OccurredAt: time.Now().UTC()}
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		signature := signPaymentPayload(handler.paymentWebhookSecret, body)
		if err := handler.processPaymentWebhook(ctx, body, signature); err != nil {
			t.Fatal(err)
		}
		if err := handler.processPaymentWebhook(ctx, body, signature); err != nil {
			t.Fatal(err)
		}

		var orders int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE reservation_id = $1`, reservationID).Scan(&orders); err != nil {
			t.Fatal(err)
		}
		if orders != 1 {
			t.Fatalf("orders = %d, want 1", orders)
		}
		var duplicateEvents int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM payment_events WHERE id=$1`, event.ID).Scan(&duplicateEvents); err != nil {
			t.Fatal(err)
		}
		if duplicateEvents != 1 {
			t.Fatalf("duplicate events = %d, want 1", duplicateEvents)
		}
	})

	t.Run("failed payment keeps the reservation available for a successful retry", func(t *testing.T) {
		reservationID := "payment-retry-" + suffix
		userID := "payment-retry-user-" + suffix
		email := "payment-retry@example.com"
		if _, err := db.Exec(ctx, `
			INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at)
			VALUES ($1, $2, 'item-1', $3, 'M', 'pending', NOW() + INTERVAL '10 minutes')
		`, reservationID, failureDrop, userID); err != nil {
			t.Fatal(err)
		}

		createPayment := func(scenario string) paymentResponse {
			request := httptest.NewRequest(http.MethodPost, "/api/checkout/"+reservationID+"/payments",
				bytes.NewBufferString(fmt.Sprintf(`{"name":"Test User","address":"Test Address","scenario":%q}`, scenario)))
			request.SetPathValue("reservationID", reservationID)
			requestCtx := context.WithValue(request.Context(), CtxUserID, userID)
			requestCtx = context.WithValue(requestCtx, CtxEmail, email)
			response := httptest.NewRecorder()
			handler.CreatePayment(response, request.WithContext(requestCtx))
			if response.Code != http.StatusAccepted {
				t.Fatalf("create %s payment: status %d: %s", scenario, response.Code, response.Body.String())
			}
			var payment paymentResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payment); err != nil {
				t.Fatal(err)
			}
			return payment
		}

		waitForStatus := func(paymentID, wanted string) {
			deadline := time.Now().Add(4 * time.Second)
			var status string
			for time.Now().Before(deadline) {
				if err := db.QueryRow(ctx, `SELECT status FROM payments WHERE id=$1`, paymentID).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status == wanted {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatalf("payment %s status = %q, want %q", paymentID, status, wanted)
		}

		declined := createPayment("declined")
		waitForStatus(declined.PaymentID, "failed")

		var reservationStatus string
		var orderCount int
		if err := db.QueryRow(ctx, `SELECT status FROM reservations WHERE id=$1`, reservationID).Scan(&reservationStatus); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE reservation_id=$1`, reservationID).Scan(&orderCount); err != nil {
			t.Fatal(err)
		}
		if reservationStatus != "pending" || orderCount != 0 {
			t.Fatalf("after decline: reservation=%q orders=%d", reservationStatus, orderCount)
		}

		succeeded := createPayment("success")
		if succeeded.PaymentID == declined.PaymentID {
			t.Fatal("retry reused a failed payment attempt")
		}
		waitForStatus(succeeded.PaymentID, "paid")

		var paymentCount int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM payments WHERE reservation_id=$1`, reservationID).Scan(&paymentCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE reservation_id=$1`, reservationID).Scan(&orderCount); err != nil {
			t.Fatal(err)
		}
		if paymentCount != 2 || orderCount != 1 {
			t.Fatalf("payments=%d orders=%d, want 2 and 1", paymentCount, orderCount)
		}
	})
}
