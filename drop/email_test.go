package drop

import (
	"context"
	"io"
	"log/slog"
	"regexp"
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

func TestBuildMIMEUsesStableMessageIDForRetries(t *testing.T) {
	first := buildMIME("from@example.com", "to@example.com", "Subject", "Body", "order-confirmation:ORD-1")
	second := buildMIME("from@example.com", "to@example.com", "Subject", "Body", "order-confirmation:ORD-1")
	pattern := regexp.MustCompile(`(?m)^Message-ID: (.+)$`)
	firstID := pattern.FindStringSubmatch(first)
	secondID := pattern.FindStringSubmatch(second)
	if len(firstID) != 2 || len(secondID) != 2 {
		t.Fatal("Message-ID header is missing")
	}
	if firstID[1] != secondID[1] {
		t.Fatalf("Message-ID changed across retries: %q != %q", firstID[1], secondID[1])
	}
}
