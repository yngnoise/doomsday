package drop

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	otpVerifyWindowSeconds = 10 * 60
	otpEmailAttemptLimit   = 10
	otpIPAttemptLimit      = 30
)

var otpVerifyRateLimitScript = redis.NewScript(`
local email_attempts = redis.call('INCR', KEYS[1])
if email_attempts == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end

local ip_attempts = redis.call('INCR', KEYS[2])
if ip_attempts == 1 then redis.call('EXPIRE', KEYS[2], ARGV[1]) end

if email_attempts > tonumber(ARGV[2]) or ip_attempts > tonumber(ARGV[3]) then
  return 0
end
return 1
`)

func otpCodeHash(email, code string) string {
	mac := hmac.New(sha256.New, jwtSecret())
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func otpRateLimitKey(kind, value string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return fmt.Sprintf("otp:verify:%s:%s", kind, hex.EncodeToString(digest[:16]))
}

func (h *OTPHandler) allowOTPVerification(ctx context.Context, email, ip string) (bool, error) {
	result, err := otpVerifyRateLimitScript.Run(
		ctx,
		h.redis,
		[]string{otpRateLimitKey("email", email), otpRateLimitKey("ip", ip)},
		otpVerifyWindowSeconds,
		otpEmailAttemptLimit,
		otpIPAttemptLimit,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("OTP verification rate limit: %w", err)
	}
	return result == 1, nil
}

func (h *OTPHandler) resetOTPEmailRateLimit(ctx context.Context, email string) {
	if err := h.redis.Del(ctx, otpRateLimitKey("email", email)).Err(); err != nil {
		h.logger.WarnContext(ctx, "OTP rate limit reset failed", "err", err)
	}
}

func requestClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
