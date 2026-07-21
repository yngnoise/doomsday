# Observability and health operations

The API emits privacy-safe JSON logs, Prometheus metrics, and OpenTelemetry traces. HTTP context is also persisted with transactional outbox jobs, so a background retry can be connected to the request that created it.

## Structured logs

Every request receives validated `X-Request-ID` and `X-Correlation-ID` values. Missing or unsafe values are replaced, and both IDs are echoed in the response. Log records created with the request or job context automatically include:

- `request_id` when the work is synchronous;
- `correlation_id` across synchronous and asynchronous work;
- `trace_id` and `span_id` when a span is active;
- bounded operational fields such as HTTP route, status, job type, attempt, and duration.

Set `LOG_LEVEL` to `debug`, `info`, `warn`, or `error`. Logs never include raw request URLs, query strings, request bodies, OTP codes, credentials, authorization values, email addresses, user IDs, customer names, or shipping addresses. Sensitive structured attributes are redacted as a second line of defense.

## Health endpoints

| Endpoint | Purpose | Failure behavior |
| --- | --- | --- |
| `GET /health/live` | Confirms that the process can serve HTTP | Does not query dependencies |
| `GET /health/ready` | Determines whether traffic should be routed to the instance | Returns `503` if PostgreSQL or Redis is unavailable |
| `GET /health/dependencies` | Shows the status and latency of each required dependency | Returns `503` with only `up`/`down` status; connection details and errors are omitted |

These endpoints have different meanings intentionally. Liveness should not restart an otherwise healthy process during a dependency outage, while readiness should remove it from traffic.

## Prometheus metrics

`GET /metrics` exposes a dedicated registry with Go/process collectors and these application metrics:

| Metric | Labels |
| --- | --- |
| `doomsday_http_requests_total` | `method`, route template, status class |
| `doomsday_http_request_duration_seconds` | `method`, route template |
| `doomsday_reservations_total` | bounded outcome |
| `doomsday_payments_total` | bounded simulator scenario and lifecycle outcome |
| `doomsday_payment_webhooks_total` | bounded event type and processing outcome |
| `doomsday_outbox_jobs_total` | bounded job type and worker outcome |

User-controlled IDs and raw paths are never metric labels. Unknown values collapse to `unknown` to prevent unbounded cardinality.

Example scrape configuration:

```yaml
scrape_configs:
  - job_name: doomsday-api
    static_configs:
      - targets: ["api:8080"]
```

## OpenTelemetry traces

The service always creates W3C Trace Context spans for HTTP requests, payment webhooks, and outbox consumers. Without an exporter, spans are intentionally dropped after providing trace IDs for correlated logs.

To export OTLP over HTTP, set:

```dotenv
OTEL_SERVICE_NAME=doomsday-api
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Standard OpenTelemetry environment variables for OTLP headers and TLS are interpreted by the official exporter. The API shuts down the provider with a bounded flush period.

When a request enqueues durable work, `traceparent`, `tracestate`, and the correlation ID are stored in `outbox_jobs` in the same transaction. The worker restores that context and starts a consumer span for every attempt, including retries recovered after a process crash.

## Suggested alerts

- readiness or dependency health remains degraded for more than two minutes;
- the `5xx` HTTP request rate increases above the normal baseline;
- `dead` outbox outcomes are non-zero;
- retry outcomes increase continuously;
- reservation dependency errors or payment webhook rejections spike.

Alert thresholds should be calibrated from hosted traffic rather than copied from a local demo.
