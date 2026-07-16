package drop

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type OTPHandler struct {
	db     *pgxpool.Pool
	redis  *redis.Client
	mailer *Mailer
	logger *slog.Logger
}

func NewOTPHandler(db *pgxpool.Pool, rdb *redis.Client, mailer *Mailer, logger *slog.Logger) *OTPHandler {
	return &OTPHandler{db: db, redis: rdb, mailer: mailer, logger: logger}
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
	if err := h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM otp_codes
		WHERE email = $1 AND created_at > NOW() - INTERVAL '10 minutes' AND used = FALSE
	`, req.Email).Scan(&recent); err != nil {
		h.logger.ErrorContext(ctx, "OTP request count failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not request code")
		return
	}
	if recent >= 3 {
		writeError(w, http.StatusTooManyRequests, "too many attempts — wait a few minutes")
		return
	}

	// Delete old unused codes for this email
	if _, err := h.db.Exec(ctx, `DELETE FROM otp_codes WHERE email = $1 AND (used = TRUE OR expires_at < NOW())`, req.Email); err != nil {
		h.logger.ErrorContext(ctx, "OTP cleanup failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not request code")
		return
	}

	// Generate 6-digit code
	code, err := generateOTP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate code")
		return
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	_, err = h.db.Exec(ctx, `
		INSERT INTO otp_codes (email, code_hash, expires_at) VALUES ($1, $2, $3)
	`, req.Email, otpCodeHash(req.Email, code), expiresAt)
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
	req.Code = strings.TrimSpace(req.Code)

	if req.Email == "" || len(req.Code) != 6 {
		writeError(w, http.StatusBadRequest, "email and 6-digit code required")
		return
	}

	allowed, err := h.allowOTPVerification(ctx, req.Email, requestClientIP(r))
	if err != nil {
		h.logger.ErrorContext(ctx, "OTP verification rate limit failed", slog.Any("err", err))
		writeError(w, http.StatusServiceUnavailable, "try again shortly")
		return
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "too many verification attempts")
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not verify code")
		return
	}
	defer tx.Rollback(ctx)

	var codeID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM otp_codes
		WHERE email = $1 AND code_hash = $2 AND used = FALSE AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, req.Email, otpCodeHash(req.Email, req.Code)).Scan(&codeID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			h.logger.ErrorContext(ctx, "OTP lookup failed", slog.Any("err", err))
		}
		writeError(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}

	candidateUserID := "u-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (id, email)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id
	`, candidateUserID, req.Email).Scan(&userID); err != nil {
		h.logger.ErrorContext(ctx, "user upsert failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not verify code")
		return
	}

	token, err := IssueUserJWT(userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	if _, err := tx.Exec(ctx, `
		UPDATE otp_codes
		SET used = TRUE
		WHERE email = $1 AND used = FALSE
	`, req.Email); err != nil {
		h.logger.ErrorContext(ctx, "OTP consumption failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not verify code")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.logger.ErrorContext(ctx, "OTP transaction commit failed", slog.Any("err", err))
		writeError(w, http.StatusInternalServerError, "could not verify code")
		return
	}

	h.resetOTPEmailRateLimit(ctx, req.Email)

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
