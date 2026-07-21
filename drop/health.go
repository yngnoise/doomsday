package drop

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type DependencyProbe func(context.Context) error

type dependencyStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

type dependencyHealthResponse struct {
	Status       string                      `json:"status"`
	Dependencies map[string]dependencyStatus `json:"dependencies"`
}

// NewDependencyHealthHandler reports each required dependency independently.
// It intentionally returns no connection details or raw error messages.
func NewDependencyHealthHandler(postgres, redis DependencyProbe) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := []struct {
			name  string
			probe DependencyProbe
		}{
			{name: "postgres", probe: postgres},
			{name: "redis", probe: redis},
		}
		response := dependencyHealthResponse{
			Status:       "healthy",
			Dependencies: make(map[string]dependencyStatus, len(checks)),
		}
		for _, check := range checks {
			started := time.Now()
			err := check.probe(ctx)
			latency := time.Since(started).Milliseconds()
			status := "up"
			if err != nil {
				status = "down"
				response.Status = "degraded"
			}
			response.Dependencies[check.name] = dependencyStatus{Status: status, LatencyMS: latency}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		statusCode := http.StatusOK
		if response.Status != "healthy" {
			statusCode = http.StatusServiceUnavailable
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	})
}
