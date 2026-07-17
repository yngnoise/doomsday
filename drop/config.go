package drop

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	minJWTSecretLength     = 32
	minAdminPasswordLength = 16
)

var insecureConfigValues = map[string]struct{}{
	"doomsday-dev-secret-change-in-prod": {},
	"doomsday-admin":                     {},
	"change_me":                          {},
	"changeme":                           {},
	"generate-a-random-secret-with-at-least-32-characters":           {},
	"generate-a-different-random-secret-with-at-least-32-characters": {},
	"generate-a-random-password-with-at-least-16-characters":         {},
}

// ValidateSecurityConfig rejects missing, weak, or documented placeholder
// credentials before the HTTP server starts.
func ValidateSecurityConfig() error {
	jwt := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	paymentWebhook := strings.TrimSpace(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	admin := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	e2eOTPCode := strings.TrimSpace(os.Getenv("E2E_OTP_CODE"))
	demoOTPCode := strings.TrimSpace(os.Getenv("DEMO_OTP_CODE"))
	demoMode := strings.EqualFold(strings.TrimSpace(os.Getenv("DEMO_MODE")), "true")
	corsOrigins := strings.TrimSpace(os.Getenv("CORS_ORIGINS"))

	var problems []string
	if err := validateSecret("JWT_SECRET", jwt, minJWTSecretLength); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateSecret("PAYMENT_WEBHOOK_SECRET", paymentWebhook, minJWTSecretLength); err != nil {
		problems = append(problems, err.Error())
	}
	if jwt != "" && paymentWebhook != "" && jwt == paymentWebhook {
		problems = append(problems, "PAYMENT_WEBHOOK_SECRET must be different from JWT_SECRET")
	}
	if err := validateSecret("ADMIN_PASSWORD", admin, minAdminPasswordLength); err != nil {
		problems = append(problems, err.Error())
	}
	if appEnv == "production" && corsOrigins == "" {
		problems = append(problems, "CORS_ORIGINS is required in production")
	}
	if e2eOTPCode != "" && appEnv != "test" {
		problems = append(problems, "E2E_OTP_CODE is only allowed when APP_ENV=test")
	}
	if e2eOTPCode != "" && (len(e2eOTPCode) != 6 || strings.Trim(e2eOTPCode, "0123456789") != "") {
		problems = append(problems, "E2E_OTP_CODE must contain exactly 6 digits")
	}
	if demoOTPCode != "" && !demoMode {
		problems = append(problems, "DEMO_OTP_CODE is only allowed when DEMO_MODE=true")
	}
	if demoMode && (len(demoOTPCode) != 6 || strings.Trim(demoOTPCode, "0123456789") != "") {
		problems = append(problems, "DEMO_OTP_CODE must contain exactly 6 digits when DEMO_MODE=true")
	}
	for _, origin := range strings.Split(corsOrigins, ",") {
		if strings.TrimSpace(origin) == "*" {
			problems = append(problems, "CORS_ORIGINS must not contain a wildcard")
			break
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateSecret(name, value string, minLength int) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) < minLength {
		return fmt.Errorf("%s must be at least %d characters", name, minLength)
	}
	if _, insecure := insecureConfigValues[strings.ToLower(value)]; insecure {
		return fmt.Errorf("%s uses an insecure placeholder", name)
	}
	return nil
}
