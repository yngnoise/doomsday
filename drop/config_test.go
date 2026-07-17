package drop

import (
	"strings"
	"testing"
)

func TestValidateSecurityConfig(t *testing.T) {
	t.Run("accepts strong credentials", func(t *testing.T) {
		t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))
		t.Setenv("PAYMENT_WEBHOOK_SECRET", strings.Repeat("p", minJWTSecretLength))
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		if err := ValidateSecurityConfig(); err != nil {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("rejects missing credentials", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		t.Setenv("PAYMENT_WEBHOOK_SECRET", "")
		t.Setenv("ADMIN_PASSWORD", "")
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "JWT_SECRET is required") || !strings.Contains(err.Error(), "PAYMENT_WEBHOOK_SECRET is required") || !strings.Contains(err.Error(), "ADMIN_PASSWORD is required") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("rejects weak credentials", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "short")
		t.Setenv("PAYMENT_WEBHOOK_SECRET", "short")
		t.Setenv("ADMIN_PASSWORD", "doomsday-admin")
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "JWT_SECRET must be at least") || !strings.Contains(err.Error(), "PAYMENT_WEBHOOK_SECRET must be at least") || !strings.Contains(err.Error(), "ADMIN_PASSWORD must be at least") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("requires explicit production CORS origins", func(t *testing.T) {
		t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))
		t.Setenv("PAYMENT_WEBHOOK_SECRET", strings.Repeat("p", minJWTSecretLength))
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		t.Setenv("APP_ENV", "production")
		t.Setenv("CORS_ORIGINS", "")
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "CORS_ORIGINS is required in production") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("rejects wildcard CORS origins", func(t *testing.T) {
		t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))
		t.Setenv("PAYMENT_WEBHOOK_SECRET", strings.Repeat("p", minJWTSecretLength))
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		t.Setenv("CORS_ORIGINS", "*")
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "must not contain a wildcard") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("requires an independent payment secret", func(t *testing.T) {
		shared := strings.Repeat("s", minJWTSecretLength)
		t.Setenv("JWT_SECRET", shared)
		t.Setenv("PAYMENT_WEBHOOK_SECRET", shared)
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "must be different") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("allows a fixed OTP only in test mode", func(t *testing.T) {
		t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))
		t.Setenv("PAYMENT_WEBHOOK_SECRET", strings.Repeat("p", minJWTSecretLength))
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		t.Setenv("E2E_OTP_CODE", "424242")
		t.Setenv("APP_ENV", "test")
		if err := ValidateSecurityConfig(); err != nil {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})

	t.Run("rejects a fixed OTP outside test mode", func(t *testing.T) {
		t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))
		t.Setenv("PAYMENT_WEBHOOK_SECRET", strings.Repeat("p", minJWTSecretLength))
		t.Setenv("ADMIN_PASSWORD", strings.Repeat("a", minAdminPasswordLength))
		t.Setenv("E2E_OTP_CODE", "424242")
		t.Setenv("APP_ENV", "production")
		t.Setenv("CORS_ORIGINS", "https://example.com")
		err := ValidateSecurityConfig()
		if err == nil || !strings.Contains(err.Error(), "only allowed") {
			t.Fatalf("ValidateSecurityConfig() error = %v", err)
		}
	})
}
