package drop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Lua script — atomically:
//  1. Rate-limit check
//  2. Duplicate reservation check
//  3. Decrement size-specific stock
//  4. Decrement total stock
//
// KEYS[1] = drop:{id}:size:{size}:stock
// KEYS[2] = drop:{id}:stock  (total)
// KEYS[3] = drop:{id}:reservations
// KEYS[4] = drop:{id}:rl:{userID}
//
// Returns: 0=ok, -1=sold out (size), -2=already reserved, -3=rate limited
const reserveScript = `
local size_key  = KEYS[1]
local total_key = KEYS[2]
local resv_key  = KEYS[3]
local rl_key    = KEYS[4]
local user_id   = ARGV[1]
local resv_id   = ARGV[2]
local rl_window = tonumber(ARGV[3])
local rl_max    = tonumber(ARGV[4])

local attempts = redis.call('INCR', rl_key)
if attempts == 1 then redis.call('EXPIRE', rl_key, rl_window) end
if attempts > rl_max then return -3 end

if redis.call('HEXISTS', resv_key, user_id) == 1 then return -2 end

local size_left = tonumber(redis.call('GET', size_key))
if size_left == nil or size_left <= 0 then return -1 end

local total_left = tonumber(redis.call('GET', total_key))
if total_left == nil or total_left <= 0 then return -1 end

redis.call('DECR', size_key)
redis.call('DECR', total_key)
redis.call('HSET', resv_key, user_id, resv_id)
return 0
`

// ─────────────────────────────────────────────────────────────────────────────
// HANDLER
// ─────────────────────────────────────────────────────────────────────────────

type Handler struct {
	redis                *redis.Client
	db                   *pgxpool.Pool
	sha                  string
	hub                  *Hub
	mailer               *Mailer
	logger               *slog.Logger
	paymentWebhookSecret string
	telemetry            *Telemetry
}

func (h *Handler) SetTelemetry(telemetry *Telemetry) {
	h.telemetry = telemetry
}

func NewHandler(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, hub *Hub, mailer *Mailer, logger *slog.Logger, paymentWebhookSecret ...string) (*Handler, error) {
	sha, err := rdb.ScriptLoad(ctx, reserveScript).Result()
	if err != nil {
		return nil, fmt.Errorf("loading Lua script: %w", err)
	}
	secret := "test-payment-webhook-secret-with-at-least-32-characters"
	if len(paymentWebhookSecret) > 0 && paymentWebhookSecret[0] != "" {
		secret = paymentWebhookSecret[0]
	}
	h := &Handler{redis: rdb, db: db, sha: sha, hub: hub, mailer: mailer, logger: logger, paymentWebhookSecret: secret}
	if err := h.initStock(ctx); err != nil {
		logger.WarnContext(ctx, "stock init warning", slog.Any("err", err))
	}
	return h, nil
}

// initStock seeds Redis from DB for every active drop (SETNX — never overwrites live counter).
func (h *Handler) initStock(ctx context.Context) error {
	// Seed total stock
	rows, err := h.db.Query(ctx, `
		SELECT d.id,
			d.total_stock - COALESCE(
				(SELECT COUNT(*) FROM reservations r
				 WHERE r.drop_id = d.id AND r.status IN ('pending','expiring','completed')), 0
			) AS remaining
		FROM drops d WHERE d.ends_at > NOW()
	`)
	if err != nil {
		return fmt.Errorf("querying active drops: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var remaining int64
		if err := rows.Scan(&id, &remaining); err != nil {
			continue
		}
		if err := h.redis.SetNX(ctx, fmt.Sprintf("drop:%s:stock", id), remaining, 0).Err(); err != nil {
			return fmt.Errorf("seed total stock for %s: %w", id, err)
		}

		// Seed per-size stock
		sizeRows, err := h.db.Query(ctx, `
			SELECT ds.label,
				ds.stock - COALESCE(
					(SELECT COUNT(*) FROM reservations r
					 WHERE r.drop_id = ds.drop_id AND r.size = ds.label AND r.status IN ('pending','expiring','completed')), 0
				) AS remaining
			FROM drop_sizes ds WHERE ds.drop_id = $1
		`, id)
		if err != nil {
			h.logger.WarnContext(ctx, "size stock query failed", slog.String("drop", id), slog.Any("err", err))
			continue
		}
		for sizeRows.Next() {
			var label string
			var sizeRemaining int64
			if err := sizeRows.Scan(&label, &sizeRemaining); err != nil {
				continue
			}
			key := fmt.Sprintf("drop:%s:size:%s:stock", id, label)
			if err := h.redis.SetNX(ctx, key, sizeRemaining, 0).Err(); err != nil {
				sizeRows.Close()
				return fmt.Errorf("seed size stock for %s/%s: %w", id, label, err)
			}
			h.logger.InfoContext(ctx, "size stock initialized",
				slog.String("drop", id), slog.String("size", label), slog.Int64("remaining", sizeRemaining))
		}
		sizeRows.Close()

		reservationRows, err := h.db.Query(ctx, `
			SELECT user_id, id FROM reservations
			WHERE drop_id = $1 AND status IN ('pending','expiring','completed')
		`, id)
		if err != nil {
			return fmt.Errorf("query reservation markers for %s: %w", id, err)
		}
		markerKey := fmt.Sprintf("drop:%s:reservations", id)
		for reservationRows.Next() {
			var userID, reservationID string
			if err := reservationRows.Scan(&userID, &reservationID); err != nil {
				reservationRows.Close()
				return fmt.Errorf("scan reservation marker for %s: %w", id, err)
			}
			if err := h.redis.HSetNX(ctx, markerKey, userID, reservationID).Err(); err != nil {
				reservationRows.Close()
				return fmt.Errorf("seed reservation marker for %s: %w", id, err)
			}
		}
		if err := reservationRows.Err(); err != nil {
			reservationRows.Close()
			return fmt.Errorf("iterate reservation markers for %s: %w", id, err)
		}
		reservationRows.Close()
	}
	return rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/drops  — public list for the archive page
// ─────────────────────────────────────────────────────────────────────────────

type DropListItem struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PriceCents     int       `json:"price_cents"`
	TotalStock     int       `json:"total_stock"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	StockRemaining int64     `json:"stock_remaining"`
	Phase          string    `json:"phase"`
}

func (h *Handler) ListDrops(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.Query(ctx, `
		SELECT id, name, price_cents, total_stock, starts_at, ends_at
		FROM drops ORDER BY starts_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	result := []DropListItem{}
	for rows.Next() {
		var d DropListItem
		if err := rows.Scan(&d.ID, &d.Name, &d.PriceCents, &d.TotalStock, &d.StartsAt, &d.EndsAt); err != nil {
			continue
		}
		d.StockRemaining, _ = h.redis.Get(ctx, fmt.Sprintf("drop:%s:stock", d.ID)).Int64()
		switch {
		case now.Before(d.StartsAt):
			d.Phase = "pre"
		case now.After(d.EndsAt):
			d.Phase = "ended"
		case d.StockRemaining == 0:
			d.Phase = "sold_out"
		default:
			d.Phase = "live"
		}
		result = append(result, d)
	}
	writeJSON(w, http.StatusOK, result)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/auth/guest
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) GuestToken(w http.ResponseWriter, r *http.Request) {
	userID, token, err := IssueGuestToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "user_id": userID})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/drops/{dropID}
// ─────────────────────────────────────────────────────────────────────────────

type SizeInfo struct {
	Label string `json:"label"`
	Stock int64  `json:"stock"`
}

type dropRecord struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int       `json:"price_cents"`
	TotalStock  int       `json:"total_stock"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
}

type GetDropResponse struct {
	dropRecord
	StockRemaining int64      `json:"stock_remaining"`
	Phase          string     `json:"phase"`
	Sizes          []SizeInfo `json:"sizes"`
}

func (h *Handler) GetDrop(w http.ResponseWriter, r *http.Request) {
	dropID := r.PathValue("dropID")
	if dropID == "" {
		writeError(w, http.StatusBadRequest, "missing drop id")
		return
	}
	d, err := h.fetchDrop(r.Context(), dropID)
	if err != nil {
		writeError(w, http.StatusNotFound, "drop not found")
		return
	}

	ctx := r.Context()
	total, _ := h.redis.Get(ctx, fmt.Sprintf("drop:%s:stock", dropID)).Int64()

	// Load per-size stock from Redis
	sizes, _ := h.fetchSizes(ctx, dropID)

	now := time.Now().UTC()
	phase := "pre"
	switch {
	case now.After(d.EndsAt):
		phase = "ended"
	case now.After(d.StartsAt) && total == 0:
		phase = "sold_out"
	case now.After(d.StartsAt):
		phase = "live"
	}

	writeJSON(w, http.StatusOK, GetDropResponse{
		dropRecord:     *d,
		StockRemaining: total,
		Phase:          phase,
		Sizes:          sizes,
	})
}

// fetchSizes loads per-size stock from Redis, falling back to DB labels if keys missing.
func (h *Handler) fetchSizes(ctx context.Context, dropID string) ([]SizeInfo, error) {
	rows, err := h.db.Query(ctx,
		`SELECT label FROM drop_sizes WHERE drop_id = $1 ORDER BY
			CASE label WHEN 'XS' THEN 1 WHEN 'S' THEN 2 WHEN 'M' THEN 3
			           WHEN 'L' THEN 4 WHEN 'XL' THEN 5 WHEN 'XXL' THEN 6 ELSE 99 END`,
		dropID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sizes []SizeInfo
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			continue
		}
		key := fmt.Sprintf("drop:%s:size:%s:stock", dropID, label)
		stock, _ := h.redis.Get(ctx, key).Int64()
		sizes = append(sizes, SizeInfo{Label: label, Stock: stock})
	}
	return sizes, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/reserve
// ─────────────────────────────────────────────────────────────────────────────

type ReserveRequest struct {
	DropID string `json:"drop_id"`
	ItemID string `json:"item_id"`
	Size   string `json:"size"`
	Email  string `json:"email"`
}

type ReserveResponse struct {
	ReservationID string    `json:"reservation_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	StockLeft     int64     `json:"stock_left"`
}

func (h *Handler) ReserveItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	outcome := "invalid"
	defer func() { h.telemetry.RecordReservation(outcome) }()
	var req ReserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DropID == "" || req.ItemID == "" {
		writeError(w, http.StatusBadRequest, "drop_id and item_id are required")
		return
	}
	if req.Size == "" {
		writeError(w, http.StatusBadRequest, "size is required")
		return
	}
	userID, ok := userIDFromContext(ctx)
	if !ok {
		outcome = "unauthenticated"
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	d, err := h.fetchDrop(ctx, req.DropID)
	if err != nil {
		outcome = "not_live"
		writeError(w, http.StatusNotFound, "drop not found")
		return
	}
	now := time.Now().UTC()
	if now.Before(d.StartsAt) {
		outcome = "not_live"
		writeError(w, http.StatusConflict, "drop has not started")
		return
	}
	if now.After(d.EndsAt) {
		outcome = "not_live"
		writeError(w, http.StatusGone, "drop has ended")
		return
	}

	reservationID := uuid.NewString()
	code, totalLeft, err := h.reserveInRedis(ctx, req.DropID, req.Size, userID, reservationID)
	if err != nil {
		outcome = "dependency_error"
		h.logger.ErrorContext(ctx, "redis reserve", slog.Any("err", err))
		writeError(w, http.StatusServiceUnavailable, "try again shortly")
		return
	}
	switch code {
	case -3:
		outcome = "rate_limited"
		writeError(w, http.StatusTooManyRequests, "too many attempts")
		return
	case -2:
		outcome = "duplicate"
		// User already has an active reservation — return it so the frontend can redirect
		var existID string
		var existExpires time.Time
		err := h.db.QueryRow(ctx, `
			SELECT id, expires_at FROM reservations
			WHERE drop_id=$1 AND user_id=$2 AND status='pending' AND expires_at > NOW()
			ORDER BY created_at DESC LIMIT 1
		`, req.DropID, userID).Scan(&existID, &existExpires)
		if err == nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":          "already reserved by this user",
				"reservation_id": existID,
				"expires_at":     existExpires.UTC().Format(time.RFC3339),
			})
			return
		}
		writeError(w, http.StatusConflict, "already reserved by this user")
		return
	case -1:
		outcome = "sold_out"
		writeError(w, http.StatusGone, fmt.Sprintf("size %s is sold out", req.Size))
		return
	}

	expiresAt := now.Add(10 * time.Minute)
	var confirmation *reservationEmailPayload
	if req.Email != "" {
		confirmation = &reservationEmailPayload{
			To: req.Email, Name: userID, ItemName: d.Name,
			ReservationID: reservationID, ExpiresAt: expiresAt,
		}
	}
	if err := h.persistReservation(ctx, reservationID, req.DropID, req.ItemID, userID, req.Size, expiresAt, confirmation); err != nil {
		outcome = "dependency_error"
		h.logger.ErrorContext(ctx, "reservation persistence failed",
			slog.String("reservation", reservationID),
			slog.Any("err", err),
		)

		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		release, releaseErr := releaseReservationInRedis(
			rollbackCtx,
			h.redis,
			req.DropID,
			req.Size,
			userID,
			reservationID,
		)
		if releaseErr != nil {
			h.logger.ErrorContext(rollbackCtx, "reservation rollback failed",
				slog.String("reservation", reservationID),
				slog.Any("err", releaseErr),
			)
		} else if release.Released {
			h.hub.Broadcast(req.DropID, release.TotalStock)
		}

		writeError(w, http.StatusServiceUnavailable, "could not create reservation")
		return
	}

	h.hub.Broadcast(req.DropID, totalLeft)
	outcome = "created"

	writeJSON(w, http.StatusCreated, ReserveResponse{
		ReservationID: reservationID,
		ExpiresAt:     expiresAt,
		StockLeft:     totalLeft,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/checkout/{reservationID}/complete
// ─────────────────────────────────────────────────────────────────────────────

type checkoutRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (req *checkoutRequest) validate(verifiedEmail string) error {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)

	if verifiedEmail == "" {
		return errors.New("verified email required")
	}
	if req.Email != "" && !strings.EqualFold(req.Email, verifiedEmail) {
		return errors.New("checkout email must match verified email")
	}
	if req.Name == "" || req.Address == "" {
		return errors.New("name and address are required")
	}
	return nil
}

func (h *Handler) CompleteCheckout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reservationID := r.PathValue("reservationID")
	userID, ok := userIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	verifiedEmail := strings.ToLower(strings.TrimSpace(emailFromContext(ctx)))
	var req checkoutRequest
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
		writeError(w, http.StatusInternalServerError, "could not complete order")
		return
	}
	defer tx.Rollback(ctx)

	var expiresAt time.Time
	var dropID, itemID, size, status, dropName string
	var priceCents int
	err = tx.QueryRow(ctx, `
		SELECT r.expires_at, r.drop_id, r.item_id, r.size, r.status,
		       d.name, d.price_cents
		FROM reservations r
		JOIN drops d ON d.id = r.drop_id
		WHERE r.id = $1 AND r.user_id = $2
		FOR UPDATE OF r
	`, reservationID, userID).Scan(
		&expiresAt,
		&dropID,
		&itemID,
		&size,
		&status,
		&dropName,
		&priceCents,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "reservation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if status == "completed" {
		var existingOrderID, existingSize string
		if err := tx.QueryRow(ctx, `
			SELECT id, size FROM orders WHERE reservation_id = $1
		`, reservationID).Scan(&existingOrderID, &existingSize); err != nil {
			h.logger.ErrorContext(ctx, "completed reservation has no order",
				slog.String("reservation", reservationID),
				slog.Any("err", err),
			)
			writeError(w, http.StatusInternalServerError, "could not load completed order")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"order_id": existingOrderID,
			"status":   "completed",
			"size":     existingSize,
		})
		return
	}
	if status != "pending" || time.Now().After(expiresAt) {
		writeError(w, http.StatusGone, "reservation expired")
		return
	}

	orderID := "ORD-" + uuid.NewString()[:8]
	if _, err := tx.Exec(ctx, `
		INSERT INTO orders (
			id, reservation_id, drop_id, item_id, user_id, size,
			email, customer_name, address, amount_cents, status
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'completed')
	`,
		orderID,
		reservationID,
		dropID,
		itemID,
		userID,
		size,
		verifiedEmail,
		req.Name,
		req.Address,
		priceCents,
	); err != nil {
		h.logger.ErrorContext(ctx, "order persistence failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not complete order")
		return
	}

	result, err := tx.Exec(ctx, `
		UPDATE reservations
		SET status = 'completed'
		WHERE id = $1 AND status = 'pending'
	`, reservationID)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, http.StatusInternalServerError, "could not complete order")
		return
	}
	if err := enqueueOutboxJob(ctx, tx, outboxJobOrderEmail, "order-confirmation:"+orderID, orderEmailPayload{
		To: verifiedEmail, Name: req.Name, ItemName: dropName, OrderID: orderID, PriceCents: priceCents,
	}); err != nil {
		h.logger.ErrorContext(ctx, "order confirmation enqueue failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not complete order")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.logger.ErrorContext(ctx, "order transaction commit failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not complete order")
		return
	}

	h.logger.InfoContext(ctx, "order completed",
		slog.String("order", orderID),
		slog.String("size", size),
	)

	writeJSON(w, http.StatusOK, map[string]string{
		"order_id": orderID,
		"status":   "completed",
		"size":     size,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/waitlist
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) JoinWaitlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := userIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req struct {
		DropID string `json:"drop_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	key := fmt.Sprintf("drop:%s:waitlist", req.DropID)
	score := float64(time.Now().UnixMilli())
	h.redis.ZAddNX(ctx, key, redis.Z{Score: score, Member: userID})
	rank, _ := h.redis.ZRank(ctx, key, userID).Result()
	writeJSON(w, http.StatusOK, map[string]any{
		"position": int(rank) + 1,
		"message":  fmt.Sprintf("You are #%d in the queue.", int(rank)+1),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// INTERNAL
// ─────────────────────────────────────────────────────────────────────────────

func (h *Handler) reserveInRedis(ctx context.Context, dropID, size, userID, reservationID string) (int64, int64, error) {
	sizeKey := fmt.Sprintf("drop:%s:size:%s:stock", dropID, size)
	totalKey := fmt.Sprintf("drop:%s:stock", dropID)
	resvKey := fmt.Sprintf("drop:%s:reservations", dropID)
	rlKey := fmt.Sprintf("drop:%s:rl:%s", dropID, userID)

	pipe := h.redis.Pipeline()
	evalCmd := pipe.EvalSha(ctx, h.sha,
		[]string{sizeKey, totalKey, resvKey, rlKey},
		userID, reservationID, "10", "3",
	)
	totalCmd := pipe.Get(ctx, totalKey)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, 0, fmt.Errorf("redis pipeline: %w", err)
	}
	code, err := evalCmd.Int64()
	if err != nil {
		return 0, 0, fmt.Errorf("evalsha: %w", err)
	}
	totalLeft, _ := totalCmd.Int64()
	return code, totalLeft, nil
}

func (h *Handler) persistReservation(ctx context.Context, id, dropID, itemID, userID, size string, expiresAt time.Time, confirmation *reservationEmailPayload) error {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		INSERT INTO reservations (id, drop_id, item_id, user_id, size, status, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,'pending',$6,NOW())
		ON CONFLICT (id) DO NOTHING
	`, id, dropID, itemID, userID, size, expiresAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("reservation %s was not inserted", id)
	}
	if confirmation != nil {
		if err := enqueueOutboxJob(ctx, tx, outboxJobReservationEmail, "reservation-confirmation:"+id, *confirmation); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (h *Handler) fetchDrop(ctx context.Context, dropID string) (*dropRecord, error) {
	cacheKey := fmt.Sprintf("drop:%s:meta", dropID)
	if raw, err := h.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var d dropRecord
		if json.Unmarshal(raw, &d) == nil {
			return &d, nil
		}
	}
	var d dropRecord
	err := h.db.QueryRow(ctx, `
		SELECT id, name, description, price_cents, total_stock, starts_at, ends_at
		FROM drops WHERE id = $1
	`, dropID).Scan(&d.ID, &d.Name, &d.Description, &d.PriceCents, &d.TotalStock, &d.StartsAt, &d.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("drop %q not found", dropID)
	}
	if err != nil {
		return nil, err
	}
	if b, err := json.Marshal(d); err == nil {
		h.redis.Set(ctx, cacheKey, b, 10*time.Second)
	}
	return &d, nil
}
