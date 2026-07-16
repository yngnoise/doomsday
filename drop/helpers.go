package drop

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WithCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !applyCORS(w, r) {
			writeError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func applyCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	configuredOrigins := strings.TrimSpace(os.Getenv("CORS_ORIGINS"))
	if configuredOrigins == "" {
		configuredOrigins = strings.TrimSuffix(strings.TrimSpace(os.Getenv("SITE_URL")), "/")
	}
	configured := strings.Split(configuredOrigins, ",")
	for _, candidate := range configured {
		if strings.TrimSpace(candidate) == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			return true
		}
	}
	return false
}
