package drop

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORS(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "https://shop.example.com, https://admin.example.com")
	handler := WithCORS(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("allows configured origin", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Origin", "https://shop.example.com")
		response := httptest.NewRecorder()
		handler(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if origin := response.Header().Get("Access-Control-Allow-Origin"); origin != "https://shop.example.com" {
			t.Fatalf("allow origin = %q", origin)
		}
	})

	t.Run("rejects unknown origin", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Origin", "https://attacker.example.com")
		response := httptest.NewRecorder()
		handler(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
		}
	})

	t.Run("supports PATCH preflight", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodOptions, "/", nil)
		request.Header.Set("Origin", "https://admin.example.com")
		response := httptest.NewRecorder()
		handler(response, request)

		if methods := response.Header().Get("Access-Control-Allow-Methods"); methods != "POST, GET, PATCH, OPTIONS" {
			t.Fatalf("allow methods = %q", methods)
		}
	})
}
