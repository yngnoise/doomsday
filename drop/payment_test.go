package drop

import (
	"strings"
	"testing"
)

func TestPaymentSignature(t *testing.T) {
	secret := strings.Repeat("s", 32)
	body := []byte(`{"id":"event-1","payment_id":"payment-1","type":"payment.succeeded"}`)
	signature := signPaymentPayload(secret, body)

	if !verifyPaymentSignature(secret, body, signature) {
		t.Fatal("valid signature was rejected")
	}
	if verifyPaymentSignature(secret, []byte(`{"tampered":true}`), signature) {
		t.Fatal("tampered payload was accepted")
	}
	if verifyPaymentSignature("different-secret", body, signature) {
		t.Fatal("wrong secret was accepted")
	}
}

func TestCreatePaymentRequestValidation(t *testing.T) {
	for _, scenario := range []string{"success", "declined", "cancelled", "timeout"} {
		req := createPaymentRequest{
			checkoutRequest: checkoutRequest{Name: "Test User", Address: "Test Address"},
			Scenario:        scenario,
		}
		if err := req.validate("user@example.com"); err != nil {
			t.Fatalf("scenario %q: %v", scenario, err)
		}
	}

	req := createPaymentRequest{
		checkoutRequest: checkoutRequest{Name: "Test User", Address: "Test Address"},
		Scenario:        "real-card",
	}
	if err := req.validate("user@example.com"); err == nil {
		t.Fatal("unknown scenario was accepted")
	}
}
