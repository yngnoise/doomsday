package drop

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "doomsday"

const (
	requestIDContextKey     contextKey = "request_id"
	correlationIDContextKey contextKey = "correlation_id"
)

type Telemetry struct {
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	reservations *prometheus.CounterVec
	payments     *prometheus.CounterVec
	webhooks     *prometheus.CounterVec
	outboxJobs   *prometheus.CounterVec
}

func NewTelemetry(ctx context.Context, logger *slog.Logger) (*Telemetry, func(context.Context) error, error) {
	registry := prometheus.NewRegistry()
	telemetry := &Telemetry{
		registry: registry,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "doomsday", Name: "http_requests_total",
			Help: "Completed HTTP requests by method, route, and status class.",
		}, []string{"method", "route", "status_class"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "doomsday", Name: "http_request_duration_seconds",
			Help:    "HTTP request duration by method and route.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		reservations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "doomsday", Name: "reservations_total",
			Help: "Reservation attempts by bounded outcome.",
		}, []string{"outcome"}),
		payments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "doomsday", Name: "payments_total",
			Help: "Payment lifecycle transitions by simulator scenario and outcome.",
		}, []string{"scenario", "outcome"}),
		webhooks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "doomsday", Name: "payment_webhooks_total",
			Help: "Payment webhook deliveries by event type and outcome.",
		}, []string{"event_type", "outcome"}),
		outboxJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "doomsday", Name: "outbox_jobs_total",
			Help: "Outbox job processing transitions by job type and outcome.",
		}, []string{"job_type", "outcome"}),
	}
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		telemetry.httpRequests,
		telemetry.httpDuration,
		telemetry.reservations,
		telemetry.payments,
		telemetry.webhooks,
		telemetry.outboxJobs,
	)

	traceProvider, err := newTraceProvider(ctx)
	if err != nil {
		return nil, nil, err
	}
	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("OpenTelemetry error", slog.Any("err", err))
	}))
	shutdown := func(shutdownCtx context.Context) error {
		return traceProvider.Shutdown(shutdownCtx)
	}
	return telemetry, shutdown, nil
}

func newTraceProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "doomsday-api"
	}
	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = "development"
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
		attribute.String("deployment.environment.name", environment),
	))
	if err != nil {
		return nil, fmt.Errorf("OpenTelemetry resource: %w", err)
	}
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != "" {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("OTLP trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	return sdktrace.NewTracerProvider(options...), nil
}

func (t *Telemetry) Handler() http.Handler {
	if t == nil || t.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(t.registry, promhttp.HandlerOpts{})
}

func (t *Telemetry) RecordReservation(outcome string) {
	if t != nil {
		t.reservations.WithLabelValues(boundedReservationOutcome(outcome)).Inc()
	}
}

func (t *Telemetry) RecordPayment(scenario, outcome string) {
	if t != nil {
		t.payments.WithLabelValues(boundedPaymentScenario(scenario), boundedPaymentOutcome(outcome)).Inc()
	}
}

func (t *Telemetry) RecordWebhook(eventType, outcome string) {
	if t != nil {
		t.webhooks.WithLabelValues(boundedWebhookType(eventType), boundedWebhookOutcome(outcome)).Inc()
	}
}

func (t *Telemetry) RecordOutbox(jobType, outcome string) {
	if t != nil {
		t.outboxJobs.WithLabelValues(boundedOutboxType(jobType), boundedOutboxOutcome(outcome)).Inc()
	}
}

func boundedReservationOutcome(value string) string {
	return boundedLabel(value, "unknown", "created", "invalid", "unauthenticated", "not_live", "rate_limited", "duplicate", "sold_out", "dependency_error")
}

func boundedPaymentScenario(value string) string {
	return boundedLabel(value, "unknown", "success", "declined", "cancelled", "timeout", "refund")
}

func boundedPaymentOutcome(value string) string {
	return boundedLabel(value, "unknown", "created", "processing", "paid", "failed", "refunded", "duplicate", "rejected")
}

func boundedWebhookType(value string) string {
	return boundedLabel(value, "unknown", "payment.processing", "payment.succeeded", "payment.declined", "payment.cancelled", "payment.timed_out", "payment.refunded")
}

func boundedWebhookOutcome(value string) string {
	return boundedLabel(value, "unknown", "processed", "duplicate", "rejected")
}

func boundedOutboxType(value string) string {
	return boundedLabel(value, "unknown", outboxJobPaymentSimulation, outboxJobReservationEmail, outboxJobOrderEmail, outboxJobReservationExpiry, outboxJobWaitlistPromotion)
}

func boundedOutboxOutcome(value string) string {
	return boundedLabel(value, "unknown", "completed", "retry", "dead", "recovered")
}

func boundedLabel(value, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func NewStructuredLogger(writer io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: parseLogLevel(os.Getenv("LOG_LEVEL"))})
	return slog.New(&telemetryLogHandler{next: handler})
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type telemetryLogHandler struct {
	next slog.Handler
}

func (h *telemetryLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *telemetryLogHandler) Handle(ctx context.Context, record slog.Record) error {
	copy := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(redactLogAttr(attr))
		return true
	})
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		copy.AddAttrs(slog.String("request_id", requestID))
	}
	if correlationID := CorrelationIDFromContext(ctx); correlationID != "" {
		copy.AddAttrs(slog.String("correlation_id", correlationID))
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		copy.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, copy)
}

func (h *telemetryLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactLogAttr(attr))
	}
	return &telemetryLogHandler{next: h.next.WithAttrs(redacted)}
}

func (h *telemetryLogHandler) WithGroup(name string) slog.Handler {
	return &telemetryLogHandler{next: h.next.WithGroup(name)}
}

func redactLogAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if isSensitiveTelemetryKey(attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		redacted := make([]slog.Attr, 0, len(group))
		for _, nested := range group {
			redacted = append(redacted, redactLogAttr(nested))
		}
		attr.Value = slog.GroupValue(redacted...)
	}
	return attr
}

func isSensitiveTelemetryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	sensitive := []string{"email", "to", "address", "authorization", "password", "secret", "token", "otp", "code", "customer_name", "user_id"}
	for _, candidate := range sensitive {
		if normalized == candidate || strings.HasSuffix(normalized, "."+candidate) {
			return true
		}
	}
	return false
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func CorrelationIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDContextKey).(string)
	return value
}

func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationIDContextKey, correlationID)
}

func ensureCorrelationID(ctx context.Context) (context.Context, string) {
	if existing := CorrelationIDFromContext(ctx); existing != "" {
		return ctx, existing
	}
	correlationID := uuid.NewString()
	return ContextWithCorrelationID(ctx, correlationID), correlationID
}

func ObservabilityMiddleware(logger *slog.Logger, telemetry *Telemetry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := validTelemetryID(r.Header.Get("X-Request-ID"))
			if requestID == "" {
				requestID = uuid.NewString()
			}
			correlationID := validTelemetryID(r.Header.Get("X-Correlation-ID"))
			if correlationID == "" {
				correlationID = requestID
			}

			parentCtx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx := context.WithValue(parentCtx, requestIDContextKey, requestID)
			ctx = ContextWithCorrelationID(ctx, correlationID)
			ctx, span := otel.Tracer(instrumentationName).Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()
			r = r.WithContext(ctx)

			w.Header().Set("X-Request-ID", requestID)
			w.Header().Set("X-Correlation-ID", correlationID)
			metrics := httpsnoop.CaptureMetrics(next, w, r)
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			status := metrics.Code
			if status == 0 {
				status = http.StatusOK
			}
			statusClass := strconv.Itoa(status/100) + "xx"
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status),
				attribute.String("request.id", requestID),
				attribute.String("correlation.id", correlationID),
			)
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			if telemetry != nil {
				telemetry.httpRequests.WithLabelValues(r.Method, route, statusClass).Inc()
				telemetry.httpDuration.WithLabelValues(r.Method, route).Observe(metrics.Duration.Seconds())
			}
			logger.InfoContext(ctx, "HTTP request completed",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("response_bytes", metrics.Written),
				slog.Duration("duration", metrics.Duration),
			)
		})
	}
}

func validTelemetryID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return ""
	}
	return value
}
