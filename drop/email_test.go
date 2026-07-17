package drop

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestWaitlistPromotionFailsWhenSMTPIsDisabled(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASS", "")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := NewMailer(logger)
	if err := mailer.SendWaitlistPromotion(context.Background(), "user@example.com", "drop-1", "Drop One"); err == nil {
		t.Fatal("SendWaitlistPromotion() succeeded without SMTP configuration")
	}
}

func TestDemoModeDisablesSMTP(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "demo@example.com")
	t.Setenv("SMTP_PASS", "secret")

	mailer := NewMailer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if mailer.Enabled() {
		t.Fatal("NewMailer() enabled SMTP in demo mode")
	}
}
