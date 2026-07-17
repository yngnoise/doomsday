package drop

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const paymentSignatureHeader = "X-Doomsday-Signature"

var validPaymentScenarios = map[string]struct{}{
	"success":   {},
	"declined":  {},
	"cancelled": {},
	"timeout":   {},
}

type paymentGateway interface {
	Start(paymentID, scenario string)
}

type simulatedPaymentGateway struct {
	logger  *slog.Logger
	deliver func(context.Context, string, string) error
}

func newSimulatedPaymentGateway(logger *slog.Logger, deliver func(context.Context, string, string) error) paymentGateway {
	return &simulatedPaymentGateway{logger: logger, deliver: deliver}
}

func (g *simulatedPaymentGateway) Start(paymentID, scenario string) {
	go g.run(paymentID, scenario)
}

type createPaymentRequest struct {
	checkoutRequest
	Scenario string `json:"scenario"`
}

func (req *createPaymentRequest) validate(verifiedEmail string) error {
	if err := req.checkoutRequest.validate(verifiedEmail); err != nil {
		return err
	}
	req.Scenario = strings.ToLower(strings.TrimSpace(req.Scenario))
	if _, ok := validPaymentScenarios[req.Scenario]; !ok {
		return errors.New("scenario must be success, declined, cancelled, or timeout")
	}
	return nil
}

type paymentResponse struct {
	PaymentID   string  `json:"payment_id"`
	Status      string  `json:"status"`
	Scenario    string  `json:"scenario"`
	AmountCents int     `json:"amount_cents"`
	Currency    string  `json:"currency"`
	FailureCode *string `json:"failure_code,omitempty"`
	OrderID     *string `json:"order_id,omitempty"`
}

type paymentEvent struct {
	ID         string    `json:"id"`
	PaymentID  string    `json:"payment_id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CreatePayment persists checkout details, then lets the simulator deliver a
// signed asynchronous event. A client response can never complete an order.
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reservationID := r.PathValue("reservationID")
	userID, ok := userIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	verifiedEmail := strings.ToLower(strings.TrimSpace(emailFromContext(ctx)))

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := req.validate(verifiedEmail); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create payment")
		return
	}
	defer tx.Rollback(ctx)

	var expiresAt time.Time
	var reservationStatus string
	var priceCents int
	if err := tx.QueryRow(ctx, `
		SELECT r.expires_at, r.status, d.price_cents
		FROM reservations r
		JOIN drops d ON d.id = r.drop_id
		WHERE r.id = $1 AND r.user_id = $2
		FOR UPDATE OF r
	`, reservationID, userID).Scan(&expiresAt, &reservationStatus, &priceCents); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "reservation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load reservation")
		return
	}

	if existing, found, err := loadPaymentResponse(ctx, tx, reservationID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load payment")
		return
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "could not load payment")
			return
		}
		writeJSON(w, http.StatusOK, existing)
		return
	}

	if reservationStatus != "pending" || time.Now().After(expiresAt) {
		writeError(w, http.StatusGone, "reservation expired")
		return
	}

	paymentID := "PAY-" + uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (
			id, reservation_id, user_id, amount_cents, currency, scenario,
			status, email, customer_name, address
		) VALUES ($1,$2,$3,$4,'USD',$5,'pending',$6,$7,$8)
	`, paymentID, reservationID, userID, priceCents, req.Scenario, verifiedEmail, req.Name, req.Address); err != nil {
		h.logger.ErrorContext(ctx, "payment persistence failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not create payment")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create payment")
		return
	}

	h.paymentGateway.Start(paymentID, req.Scenario)
	writeJSON(w, http.StatusAccepted, paymentResponse{
		PaymentID: paymentID, Status: "pending", Scenario: req.Scenario,
		AmountCents: priceCents, Currency: "USD",
	})
}

type paymentQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadPaymentResponse(ctx context.Context, q paymentQuery, reservationID, userID string) (paymentResponse, bool, error) {
	var result paymentResponse
	var failureCode, orderID *string
	err := q.QueryRow(ctx, `
		SELECT p.id, p.status, p.scenario, p.amount_cents, p.currency,
		       p.failure_code, o.id
		FROM payments p
		LEFT JOIN orders o ON o.payment_id = p.id
		WHERE p.reservation_id = $1 AND p.user_id = $2
		  AND p.status IN ('pending', 'processing', 'paid', 'refunded')
		ORDER BY p.created_at DESC
		LIMIT 1
	`, reservationID, userID).Scan(
		&result.PaymentID, &result.Status, &result.Scenario, &result.AmountCents,
		&result.Currency, &failureCode, &orderID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentResponse{}, false, nil
	}
	result.FailureCode = failureCode
	result.OrderID = orderID
	return result, err == nil, err
}

func (h *Handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := userIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var result paymentResponse
	var failureCode, orderID *string
	err := h.db.QueryRow(ctx, `
		SELECT p.id, p.status, p.scenario, p.amount_cents, p.currency,
		       p.failure_code, o.id
		FROM payments p
		LEFT JOIN orders o ON o.payment_id = p.id
		WHERE p.id = $1 AND p.user_id = $2
	`, r.PathValue("paymentID"), userID).Scan(
		&result.PaymentID, &result.Status, &result.Scenario, &result.AmountCents,
		&result.Currency, &failureCode, &orderID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load payment")
		return
	}
	result.FailureCode = failureCode
	result.OrderID = orderID
	writeJSON(w, http.StatusOK, result)
}

func (g *simulatedPaymentGateway) run(paymentID, scenario string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	time.Sleep(250 * time.Millisecond)
	if err := g.deliver(ctx, paymentID, "payment.processing"); err != nil {
		g.logger.ErrorContext(ctx, "simulated payment processing event failed", slog.String("payment", paymentID), slog.Any("err", err))
		return
	}

	delay := 450 * time.Millisecond
	eventType := "payment.succeeded"
	switch scenario {
	case "declined":
		eventType = "payment.declined"
	case "cancelled":
		eventType = "payment.cancelled"
	case "timeout":
		eventType = "payment.timed_out"
		delay = 1500 * time.Millisecond
	}
	time.Sleep(delay)
	if err := g.deliver(ctx, paymentID, eventType); err != nil {
		g.logger.ErrorContext(ctx, "simulated payment final event failed", slog.String("payment", paymentID), slog.Any("err", err))
	}
}

func (h *Handler) deliverSimulatedEvent(ctx context.Context, paymentID, eventType string) error {
	event := paymentEvent{ID: "EVT-" + uuid.NewString(), PaymentID: paymentID, Type: eventType, OccurredAt: time.Now().UTC()}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return h.processPaymentWebhook(ctx, body, signPaymentPayload(h.paymentWebhookSecret, body))
}

func signPaymentPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyPaymentSignature(secret string, body []byte, signature string) bool {
	expected := signPaymentPayload(secret, body)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

func (h *Handler) PaymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	if err := h.processPaymentWebhook(r.Context(), body, r.Header.Get(paymentSignatureHeader)); err != nil {
		if errors.Is(err, errInvalidPaymentSignature) {
			writeError(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
		h.logger.ErrorContext(r.Context(), "payment webhook failed", slog.Any("err", err))
		writeError(w, http.StatusBadRequest, "invalid payment event")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var errInvalidPaymentSignature = errors.New("invalid payment signature")

func (h *Handler) processPaymentWebhook(ctx context.Context, body []byte, signature string) error {
	if !verifyPaymentSignature(h.paymentWebhookSecret, body, signature) {
		return errInvalidPaymentSignature
	}
	var event paymentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("decoding payment event: %w", err)
	}
	if event.ID == "" || event.PaymentID == "" || event.Type == "" {
		return errors.New("payment event fields are required")
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		INSERT INTO payment_events (id, payment_id, event_type, payload)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO NOTHING
	`, event.ID, event.PaymentID, event.Type, string(body))
	if err != nil {
		return fmt.Errorf("persisting payment event: %w", err)
	}
	if result.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	var status, reservationID, reservationStatus, dropID, itemID, userID, size string
	var email, customerName, address, dropName string
	var expiresAt time.Time
	var amountCents int
	if err := tx.QueryRow(ctx, `
		SELECT p.status, p.reservation_id, p.user_id, p.email, p.customer_name,
		       p.address, p.amount_cents, r.status, r.expires_at, r.drop_id,
		       r.item_id, r.size, d.name
		FROM payments p
		JOIN reservations r ON r.id = p.reservation_id
		JOIN drops d ON d.id = r.drop_id
		WHERE p.id = $1
		FOR UPDATE OF p, r
	`, event.PaymentID).Scan(
		&status, &reservationID, &userID, &email, &customerName, &address,
		&amountCents, &reservationStatus, &expiresAt, &dropID, &itemID, &size, &dropName,
	); err != nil {
		return fmt.Errorf("loading payment: %w", err)
	}

	var orderID string
	var sendConfirmation bool
	switch event.Type {
	case "payment.processing":
		if status == "pending" {
			_, err = tx.Exec(ctx, `UPDATE payments SET status='processing', updated_at=NOW() WHERE id=$1`, event.PaymentID)
		}
	case "payment.succeeded":
		if status == "paid" || status == "refunded" {
			break
		}
		if status == "failed" {
			return errors.New("failed payment cannot succeed")
		}
		if reservationStatus != "pending" || time.Now().After(expiresAt) {
			_, err = tx.Exec(ctx, `UPDATE payments SET status='failed', failure_code='reservation_expired', updated_at=NOW() WHERE id=$1`, event.PaymentID)
			break
		}
		orderID = "ORD-" + uuid.NewString()[:8]
		if _, err = tx.Exec(ctx, `
			INSERT INTO orders (
				id, reservation_id, drop_id, item_id, user_id, size, email,
				customer_name, address, amount_cents, status, payment_id
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'completed',$11)
		`, orderID, reservationID, dropID, itemID, userID, size, email, customerName, address, amountCents, event.PaymentID); err != nil {
			return fmt.Errorf("creating paid order: %w", err)
		}
		if _, err = tx.Exec(ctx, `UPDATE reservations SET status='completed' WHERE id=$1 AND status='pending'`, reservationID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE payments SET status='paid', failure_code=NULL, updated_at=NOW() WHERE id=$1`, event.PaymentID); err != nil {
			return err
		}
		sendConfirmation = true
	case "payment.declined", "payment.cancelled", "payment.timed_out":
		if status == "pending" || status == "processing" {
			failureCode := strings.TrimPrefix(event.Type, "payment.")
			_, err = tx.Exec(ctx, `UPDATE payments SET status='failed', failure_code=$2, updated_at=NOW() WHERE id=$1`, event.PaymentID, failureCode)
		}
	case "payment.refunded":
		if status != "paid" && status != "refunded" {
			return errors.New("only a paid payment can be refunded")
		}
		if status == "paid" {
			if _, err = tx.Exec(ctx, `UPDATE orders SET status='refunded' WHERE payment_id=$1`, event.PaymentID); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE payments SET status='refunded', updated_at=NOW() WHERE id=$1`, event.PaymentID)
		}
	default:
		return fmt.Errorf("unsupported payment event type %q", event.Type)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if sendConfirmation {
		h.mailer.SendOrderConfirmation(ctx, email, customerName, dropName, orderID, amountCents)
	}
	return nil
}

func (h *Handler) RefundPayment(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("paymentID")
	if err := h.deliverSimulatedEvent(r.Context(), paymentID, "payment.refunded"); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"payment_id": paymentID, "status": "refunded"})
}
