package drop

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestOTPCodeHash(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("s", minJWTSecretLength))

	first := otpCodeHash("User@Example.com", "123456")
	second := otpCodeHash(" user@example.com ", "123456")
	if first != second {
		t.Fatal("normalized email produced a different OTP hash")
	}
	if first == "123456" || strings.Contains(first, "123456") {
		t.Fatal("OTP hash contains the plaintext code")
	}
	if first == otpCodeHash("user@example.com", "654321") {
		t.Fatal("different OTP codes produced the same hash")
	}
}

func TestOTPVerificationRateLimit(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	handler := &OTPHandler{redis: client}
	ctx := context.Background()
	for attempt := 1; attempt <= otpEmailAttemptLimit; attempt++ {
		allowed, err := handler.allowOTPVerification(ctx, "user@example.com", "192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Fatalf("attempt %d was blocked before the configured limit", attempt)
		}
	}

	allowed, err := handler.allowOTPVerification(ctx, "user@example.com", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("attempt above the email limit was allowed")
	}
}

func TestRequestClientIP(t *testing.T) {
	request := &http.Request{RemoteAddr: "192.0.2.15:54321"}
	if got := requestClientIP(request); got != "192.0.2.15" {
		t.Fatalf("requestClientIP() = %q, want 192.0.2.15", got)
	}
}
