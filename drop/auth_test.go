package drop

import (
	"strings"
	"testing"
)

func TestIssueUserJWTUsesStableUserID(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("j", minJWTSecretLength))

	const userID = "u-stable-user-id"
	token, err := IssueUserJWT(userID, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := parseClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != userID {
		t.Fatalf("token user ID = %q, want %q", claims.UserID, userID)
	}
	if claims.Email != "user@example.com" || claims.Role != "user" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
