package drop

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OTPHandler struct {
	db     *pgxpool.Pool
	mailer *Mailer
	logger *slog.Logger
}

func NewOTPHandler(db *pgxpool.Pool, mailer *Mailer, logger *slog.Logger) *OTPHandler {
	return &OTPHandler{db: db, mailer: mailer, logger: logger}
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/auth/request-otp
// Body: { "email": "user@example.com" }
// ─────────────────────────────────────────────────────────────────────────────

func (h *OTPHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email required")
		return
	}

	// Rate limit — max 3 active OTPs per email per 10 minutes
	var recent int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM otp_codes
		WHERE email = $1 AND created_at > NOW() - INTERVAL '10 minutes' AND used = FALSE
	`, req.Email).Scan(&recent)
	if recent >= 3 {
		writeError(w, http.StatusTooManyRequests, "too many attempts — wait a few minutes")
		return
	}

	// Delete old unused codes for this email
	h.db.Exec(ctx, `DELETE FROM otp_codes WHERE email = $1 AND (used = TRUE OR expires_at < NOW())`, req.Email)

	// Generate 6-digit code
	code, err := generateOTP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate code")
		return
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = h.db.Exec(ctx, `
		INSERT INTO otp_codes (email, code, expires_at) VALUES ($1, $2, $3)
	`, req.Email, code, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save code")
		return
	}

	// Send email
	h.mailer.SendOTP(ctx, req.Email, code)
	h.logger.Info("OTP issued", slog.String("email", req.Email))

	// Return masked email so frontend can display "Sent to a***@gmail.com"
	writeJSON(w, http.StatusOK, map[string]any{
		"masked_email": maskEmail(req.Email),
		"expires_in":   600, // seconds
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/auth/verify-otp
// Body: { "email": "user@example.com", "code": "384921" }
// ─────────────────────────────────────────────────────────────────────────────

func (h *OTPHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code  = strings.TrimSpace(req.Code)

	if req.Email == "" || len(req.Code) != 6 {
		writeError(w, http.StatusBadRequest, "email and 6-digit code required")
		return
	}

	// Find valid unused code
	var id string
	var expiresAt time.Time
	err := h.db.QueryRow(ctx, `
		SELECT id, expires_at FROM otp_codes
		WHERE email = $1 AND code = $2 AND used = FALSE AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1
	`, req.Email, req.Code).Scan(&id, &expiresAt)

	if err != nil {
		// Don't reveal whether email exists or code is wrong
		writeError(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}

	// Mark as used
	h.db.Exec(ctx, `UPDATE otp_codes SET used = TRUE WHERE id = $1`, id)

	// Issue user JWT
	token, err := IssueUserJWT(req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	h.logger.Info("OTP verified", slog.String("email", req.Email))
	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
		"email": req.Email,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func generateOTP() (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[0:1] + "***@" + parts[1]
	}
	return string(local[0]) + "***@" + parts[1]
}