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
