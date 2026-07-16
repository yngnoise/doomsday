package drop

import "testing"

func TestCheckoutRequestValidation(t *testing.T) {
	tests := []struct {
		name          string
		request       checkoutRequest
		verifiedEmail string
		wantError     bool
	}{
		{
			name:          "accepts verified email",
			request:       checkoutRequest{Email: "USER@example.com", Name: " User ", Address: " Address "},
			verifiedEmail: "user@example.com",
		},
		{
			name:          "accepts omitted form email",
			request:       checkoutRequest{Name: "User", Address: "Address"},
			verifiedEmail: "user@example.com",
		},
		{
			name:          "rejects a different form email",
			request:       checkoutRequest{Email: "attacker@example.com", Name: "User", Address: "Address"},
			verifiedEmail: "user@example.com",
			wantError:     true,
		},
		{
			name:          "requires shipping fields",
			request:       checkoutRequest{Email: "user@example.com"},
			verifiedEmail: "user@example.com",
			wantError:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.request.validate(test.verifiedEmail)
			if (err != nil) != test.wantError {
				t.Fatalf("validate() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}
