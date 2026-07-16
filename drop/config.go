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
}

// ValidateSecurityConfig rejects missing, weak, or documented placeholder
// credentials before the HTTP server starts.
func ValidateSecurityConfig() error {
	jwt := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	admin := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))

	var problems []string
	if err := validateSecret("JWT_SECRET", jwt, minJWTSecretLength); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateSecret("ADMIN_PASSWORD", admin, minAdminPasswordLength); err != nil {
		problems = append(problems, err.Error())
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
