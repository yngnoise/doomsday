package drop

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type AdminHandler struct {
	redis  *redis.Client
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewAdminHandler(rdb *redis.Client, db *pgxpool.Pool, logger *slog.Logger) *AdminHandler {
	return &AdminHandler{redis: rdb, db: db, logger: logger}
}

func adminPassword() string {
	return os.Getenv("ADMIN_PASSWORD")
}

// ─── POST /api/admin/login ────────────────────────────────────────────────────

func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Password != adminPassword() {
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	token, err := IssueAdminJWT()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ─── GET /api/admin/stats ─────────────────────────────────────────────────────

type StatsResponse struct {
	ActiveDropID    string     `json:"active_drop_id"`
	ActiveDropName  string     `json:"active_drop_name"`
	ActiveDropStart *time.Time `json:"active_drop_starts_at"`
	ActiveDropEnd   *time.Time `json:"active_drop_ends_at"`
	StockRemaining  int64      `json:"stock_remaining"`
	TotalStock      int        `json:"total_stock"`
	TotalOrders     int        `json:"total_orders"`
	PendingOrders   int        `json:"pending_orders"`
	ExpiredOrders   int        `json:"expired_orders"`
	CompletedOrders int        `json:"completed_orders"`
	RevenueCents    int        `json:"revenue_cents"`
}

func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var s StatsResponse

	var dropID, dropName string
	var startsAt, endsAt time.Time
	var totalStock int
	err := h.db.QueryRow(ctx, `
		SELECT id, name, starts_at, ends_at, total_stock FROM drops
		WHERE ends_at > NOW() ORDER BY starts_at ASC LIMIT 1
	`).Scan(&dropID, &dropName, &startsAt, &endsAt, &totalStock)
	if err == nil {
		s.ActiveDropID = dropID
		s.ActiveDropName = dropName
		s.ActiveDropStart = &startsAt
		s.ActiveDropEnd = &endsAt
		s.TotalStock = totalStock
		s.StockRemaining, _ = h.redis.Get(ctx, fmt.Sprintf("drop:%s:stock", dropID)).Int64()
	}

	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM reservations`).Scan(&s.TotalOrders)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM reservations WHERE status='pending'`).Scan(&s.PendingOrders)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM reservations WHERE status='expired'`).Scan(&s.ExpiredOrders)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM reservations WHERE status='completed'`).Scan(&s.CompletedOrders)
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(d.price_cents),0)
		FROM reservations r JOIN drops d ON d.id = r.drop_id
		WHERE r.status = 'completed'
	`).Scan(&s.RevenueCents)

	writeJSON(w, http.StatusOK, s)
}

// ─── GET /api/admin/drops ─────────────────────────────────────────────────────

type DropRow struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	PriceCents  int        `json:"price_cents"`
	TotalStock  int        `json:"total_stock"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	CreatedAt   time.Time  `json:"created_at"`
	StockLeft   int64      `json:"stock_remaining"`
	Completed   int        `json:"completed_orders"`
	Sizes       []SizeInfo `json:"sizes"`
}

func (h *AdminHandler) ListDrops(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.Query(ctx, `
		SELECT d.id, d.name, d.description, d.price_cents, d.total_stock,
		       d.starts_at, d.ends_at, d.created_at,
		       COUNT(r.id) FILTER (WHERE r.status='completed') AS completed
		FROM drops d
		LEFT JOIN reservations r ON r.drop_id = d.id
		GROUP BY d.id ORDER BY d.starts_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	result := []DropRow{}
	for rows.Next() {
		var dr DropRow
		if err := rows.Scan(&dr.ID, &dr.Name, &dr.Description, &dr.PriceCents,
			&dr.TotalStock, &dr.StartsAt, &dr.EndsAt, &dr.CreatedAt, &dr.Completed); err != nil {
			continue
		}
		dr.StockLeft, _ = h.redis.Get(ctx, fmt.Sprintf("drop:%s:stock", dr.ID)).Int64()

		// Load size breakdown
		sizeRows, _ := h.db.Query(ctx,
			`SELECT label FROM drop_sizes WHERE drop_id=$1 ORDER BY
				CASE label WHEN 'XS' THEN 1 WHEN 'S' THEN 2 WHEN 'M' THEN 3
				           WHEN 'L' THEN 4 WHEN 'XL' THEN 5 WHEN 'XXL' THEN 6 ELSE 99 END`,
			dr.ID)
		if sizeRows != nil {
			for sizeRows.Next() {
				var label string
				sizeRows.Scan(&label)
				stock, _ := h.redis.Get(ctx, fmt.Sprintf("drop:%s:size:%s:stock", dr.ID, label)).Int64()
				dr.Sizes = append(dr.Sizes, SizeInfo{Label: label, Stock: stock})
			}
			sizeRows.Close()
		}
		result = append(result, dr)
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── POST /api/admin/drops ────────────────────────────────────────────────────
// Sizes are passed as a string slice. Stock is split evenly.
// If total_stock is not divisible evenly, remainder goes to the last size.

func (h *AdminHandler) CreateDrop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		PriceCents  int      `json:"price_cents"`
		TotalStock  int      `json:"total_stock"`
		Sizes       []string `json:"sizes"`
		StartsAt    string   `json:"starts_at"`
		EndsAt      string   `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" || req.TotalStock <= 0 || req.PriceCents <= 0 {
		writeError(w, http.StatusBadRequest, "name, price_cents and total_stock are required")
		return
	}
	if len(req.Sizes) == 0 {
		req.Sizes = []string{"XS", "S", "M", "L", "XL", "XXL"}
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid starts_at — use RFC3339")
		return
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ends_at — use RFC3339")
		return
	}

	id := "dmsdy-" + uuid.NewString()[:8]

	// Transaction: insert drop + sizes
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO drops (id, name, description, price_cents, total_stock, starts_at, ends_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, id, req.Name, req.Description, req.PriceCents, req.TotalStock, startsAt, endsAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create drop")
		return
	}

	// Split stock evenly, remainder to last size
	perSize := req.TotalStock / len(req.Sizes)
	remainder := req.TotalStock % len(req.Sizes)
	for i, size := range req.Sizes {
		stock := perSize
		if i == len(req.Sizes)-1 {
			stock += remainder
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO drop_sizes (drop_id, label, stock) VALUES ($1,$2,$3)`,
			id, size, stock,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create sizes")
			return
		}
		// Seed Redis
		h.redis.Set(ctx, fmt.Sprintf("drop:%s:size:%s:stock", id, size), stock, 0)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "tx commit failed")
		return
	}

	// Seed total Redis stock
	h.redis.Set(ctx, fmt.Sprintf("drop:%s:stock", id), req.TotalStock, 0)
	h.logger.Info("drop created", slog.String("id", id), slog.String("name", req.Name))
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// ─── PATCH /api/admin/drops/{dropID}/timer ────────────────────────────────────

func (h *AdminHandler) ResetTimer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dropID := r.PathValue("dropID")
	var req struct {
		StartsInMinutes int `json:"starts_in_minutes"`
		DurationMinutes int `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.DurationMinutes <= 0 {
		writeError(w, http.StatusBadRequest, "duration_minutes must be > 0")
		return
	}
	startsAt := time.Now().Add(time.Duration(req.StartsInMinutes) * time.Minute)
	endsAt := startsAt.Add(time.Duration(req.DurationMinutes) * time.Minute)
	_, err := h.db.Exec(ctx, `UPDATE drops SET starts_at=$1, ends_at=$2 WHERE id=$3`, startsAt, endsAt, dropID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	h.redis.Del(ctx, fmt.Sprintf("drop:%s:meta", dropID))
	writeJSON(w, http.StatusOK, map[string]any{"starts_at": startsAt, "ends_at": endsAt})
}

// ─── PATCH /api/admin/drops/{dropID}/stock ────────────────────────────────────

func (h *AdminHandler) ResetStock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dropID := r.PathValue("dropID")
	var req struct {
		Stock int `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Stock < 0 {
		writeError(w, http.StatusBadRequest, "stock must be >= 0")
		return
	}
	h.redis.Set(ctx, fmt.Sprintf("drop:%s:stock", dropID), req.Stock, 0)
	writeJSON(w, http.StatusOK, map[string]int{"stock": req.Stock})
}

// ─── GET /api/admin/orders ────────────────────────────────────────────────────

type OrderRow struct {
	ID        string    `json:"id"`
	DropID    string    `json:"drop_id"`
	DropName  string    `json:"drop_name"`
	UserID    string    `json:"user_id"`
	Size      string    `json:"size"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *AdminHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	dropID := r.URL.Query().Get("drop_id")

	query := `
		SELECT r.id, r.drop_id, COALESCE(d.name,'?'), r.user_id, r.size,
		       r.status, r.expires_at, r.created_at
		FROM reservations r
		LEFT JOIN drops d ON d.id = r.drop_id
		WHERE 1=1
	`
	args := []any{}
	n := 1
	if status != "" {
		query += fmt.Sprintf(" AND r.status=$%d", n)
		args = append(args, status)
		n++
	}
	if dropID != "" {
		query += fmt.Sprintf(" AND r.drop_id=$%d", n)
		args = append(args, dropID)
	}
	query += " ORDER BY r.created_at DESC LIMIT 200"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	result := []OrderRow{}
	for rows.Next() {
		var o OrderRow
		if err := rows.Scan(&o.ID, &o.DropID, &o.DropName, &o.UserID, &o.Size,
			&o.Status, &o.ExpiresAt, &o.CreatedAt); err != nil {
			continue
		}
		result = append(result, o)
	}
	writeJSON(w, http.StatusOK, result)
}
