package drop

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	CtxUserID contextKey = "user_id"
	CtxEmail  contextKey = "email"
	CtxRole   contextKey = "role"
)

func userIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(CtxUserID).(string)
	return v, ok
}

func emailFromContext(ctx context.Context) string {
	v, _ := ctx.Value(CtxEmail).(string)
	return v
}

// ─────────────────────────────────────────────────────────────────────────────
// JWT SECRET
// ─────────────────────────────────────────────────────────────────────────────

func jwtSecret() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

// ─────────────────────────────────────────────────────────────────────────────
// CLAIMS
// ─────────────────────────────────────────────────────────────────────────────

type dmsdyClaims struct {
	UserID string `json:"uid"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role"` // "guest" | "user" | "admin"
	jwt.RegisteredClaims
}

func signToken(claims dmsdyClaims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
}

func parseClaims(tokenStr string) (*dmsdyClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &dmsdyClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*dmsdyClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid claims")
	}
	return claims, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TOKEN ISSUERS
// ─────────────────────────────────────────────────────────────────────────────

// IssueGuestToken — anonymous browsing, 24h.
func IssueGuestToken() (userID string, tokenStr string, err error) {
	userID = "g-" + uuid.NewString()[:12]
	tokenStr, err = signToken(dmsdyClaims{
		UserID: userID,
		Role:   "guest",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	return
}

// IssueUserJWT issues a token for a stable user record, valid for 30 days.
func IssueUserJWT(userID, email string) (tokenStr string, err error) {
	tokenStr, err = signToken(dmsdyClaims{
		UserID: userID,
		Email:  email,
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	return
}

// IssueAdminJWT — admin token, 7 days.
func IssueAdminJWT() (string, error) {
	return signToken(dmsdyClaims{
		UserID: "admin",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
}

// ParseAdminToken validates token and asserts role=admin.
func ParseAdminToken(tokenStr string) (*dmsdyClaims, error) {
	claims, err := parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Role != "admin" {
		return nil, errors.New("not an admin token")
	}
	return claims, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MIDDLEWARES
// ─────────────────────────────────────────────────────────────────────────────

// AuthMiddleware — accepts guest, user, or admin JWT. Injects uid/email/role.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return WithCORS(func(w http.ResponseWriter, r *http.Request) {
		claims, err := claimsFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxEmail, claims.Email)
		ctx = context.WithValue(ctx, CtxRole, claims.Role)
		next(w, r.WithContext(ctx))
	})
}

// UserAuthMiddleware — requires role=user or role=admin (not guest).
func UserAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return WithCORS(func(w http.ResponseWriter, r *http.Request) {
		claims, err := claimsFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if claims.Role == "guest" {
			writeError(w, http.StatusForbidden, "email verification required")
			return
		}
		ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxEmail, claims.Email)
		ctx = context.WithValue(ctx, CtxRole, claims.Role)
		next(w, r.WithContext(ctx))
	})
}

// AdminAuthMiddleware — requires role=admin.
func AdminAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return WithCORS(func(w http.ResponseWriter, r *http.Request) {
		claims, err := claimsFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if claims.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
		next(w, r.WithContext(ctx))
	})
}

func claimsFromRequest(r *http.Request) (*dmsdyClaims, error) {
	h := r.Header.Get("Authorization")
	if h == "" || !strings.HasPrefix(h, "Bearer ") {
		return nil, errors.New("no token")
	}
	return parseClaims(strings.TrimPrefix(h, "Bearer "))
}
