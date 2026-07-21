package drop

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestStructuredLoggerAddsContextAndRedactsSensitiveAttributes(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	var output bytes.Buffer
	logger := NewStructuredLogger(&output)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	ctx = context.WithValue(ctx, requestIDContextKey, "req-123")
	ctx = ContextWithCorrelationID(ctx, "corr-456")

	logger.InfoContext(ctx, "safe event",
		slog.String("email", "person@example.com"),
		slog.String("otp", "123456"),
		slog.String("job_id", "job-789"),
		slog.Group("customer", slog.String("address", "secret street")),
	)

	logged := output.String()
	for _, secret := range []string{"person@example.com", "123456", "secret street"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("sensitive value %q leaked in %s", secret, logged)
		}
	}
	for _, expected := range []string{"[REDACTED]", "req-123", "corr-456", traceID.String(), "job-789"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("expected %q in %s", expected, logged)
		}
	}
}

func TestObservabilityMiddlewarePropagatesIDsAndRecordsBoundedRouteMetrics(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	var output bytes.Buffer
	logger := NewStructuredLogger(&output)
	telemetry, shutdown, err := NewTelemetry(context.Background(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" || CorrelationIDFromContext(r.Context()) != "checkout-flow" {
			t.Fatalf("request context IDs were not propagated")
		}
		if !trace.SpanContextFromContext(r.Context()).IsValid() {
			t.Fatal("request span is missing")
		}
		w.WriteHeader(http.StatusCreated)
	})
	handler := ObservabilityMiddleware(logger, telemetry)(mux)
	request := httptest.NewRequest(http.MethodGet, "/orders/42?email=person@example.com", nil)
	request.Header.Set("X-Request-ID", "invalid header value")
	request.Header.Set("X-Correlation-ID", "checkout-flow")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "invalid header value" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("unsafe request ID was not replaced: %q", response.Header().Get("X-Request-ID"))
	}
	if strings.Contains(output.String(), "person@example.com") || strings.Contains(output.String(), "/orders/42") {
		t.Fatalf("raw URL or query leaked in log: %s", output.String())
	}

	metrics := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metrics.Body.String()
	if !strings.Contains(body, `doomsday_http_requests_total{method="GET",route="GET /orders/{id}",status_class="2xx"} 1`) {
		t.Fatalf("expected route metric, got %s", body)
	}
}

func TestDependencyHealthHandlerReportsPerDependencyWithoutErrors(t *testing.T) {
	postgresError := errors.New("postgres://user:password@private-host/database")
	handler := NewDependencyHealthHandler(
		func(context.Context) error { return postgresError },
		func(context.Context) error { return nil },
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/dependencies", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, expected := range []string{`"status":"degraded"`, `"postgres":{"status":"down"`, `"redis":{"status":"up"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in %s", expected, body)
		}
	}
	if strings.Contains(body, "password") || strings.Contains(body, "private-host") {
		t.Fatalf("dependency error leaked in %s", body)
	}
}

func TestTelemetryLabelsFallBackToBoundedValues(t *testing.T) {
	if got := boundedPaymentScenario("user-controlled-value"); got != "unknown" {
		t.Fatalf("scenario = %q", got)
	}
	if got := boundedWebhookType("payment.user-controlled-value"); got != "unknown" {
		t.Fatalf("webhook type = %q", got)
	}
	if got := boundedOutboxType("job-with-random-id"); got != "unknown" {
		t.Fatalf("outbox type = %q", got)
	}
}
