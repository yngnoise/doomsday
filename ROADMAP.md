# Doomsday Portfolio Roadmap

This roadmap turns Doomsday from a local MVP into a public, production-style portfolio project. The payment flow remains a simulation: no real card data is collected and no real money is charged.

Progress is tracked in the umbrella issue [#27](https://github.com/yngnoise/doomsday/issues/27).

## Phase 1 — Payment architecture

- [x] [Build a production-style payment gateway simulator](https://github.com/yngnoise/doomsday/issues/18)
  - Persist payments and payment events
  - Support success, decline, cancellation, timeout, and refund scenarios
  - Deliver asynchronous signed webhooks
  - Make event processing idempotent under duplicate delivery
  - Keep the gateway behind an adapter for a future real provider
  - Add automated tests and an explicit simulation notice in the UI

## Phase 2 — Public product experience

- [ ] [Deploy a safe public demo environment](https://github.com/yngnoise/doomsday/issues/19)
  - Provide a stable public URL and repeatable deployment
  - Seed and automatically restore demo data
  - Keep secrets in the hosting environment
  - Disable real email and external payment side effects
- [x] [Make the storefront responsive and accessible](https://github.com/yngnoise/doomsday/issues/20)
  - Support mobile, tablet, and desktop layouts
  - Improve keyboard navigation, focus states, semantics, and contrast
  - Cover loading, empty, error, reconnecting, and disabled states
- [ ] [Improve live drop UX and admin operations](https://github.com/yngnoise/doomsday/issues/25)
  - Show reservation countdown, connection health, and waitlist position
  - Recover cleanly after SSE disconnects
  - Add operational views for orders, payments, refunds, reservations, and failed jobs

## Phase 3 — Reliability and evidence

- [x] [Add end-to-end coverage for critical user journeys](https://github.com/yngnoise/doomsday/issues/21)
  - Exercise OTP sign-in, reservation, payment, confirmation, expiry, waitlist, and refund
  - Run the complete stack with PostgreSQL and Redis in CI
  - Retain traces and screenshots for failed scenarios
- [x] [Introduce a transactional outbox and background worker](https://github.com/yngnoise/doomsday/issues/22)
  - Persist asynchronous jobs with business transactions
  - Add retries, exponential backoff, idempotency keys, and dead-letter handling
  - Make scheduled reservation expiry safe across multiple instances
- [x] [Add production-grade observability and health checks](https://github.com/yngnoise/doomsday/issues/23)
  - Add structured logs, correlation IDs, metrics, and traces
  - Separate liveness, readiness, and dependency health
  - Ensure telemetry never exposes secrets, OTP codes, or personal data
- [ ] [Validate concurrency and resilience with load and failure testing](https://github.com/yngnoise/doomsday/issues/24)
  - Add k6 contention, checkout, and SSE scenarios
  - Verify stock and order invariants after every run
  - Demonstrate recovery from temporary PostgreSQL and Redis failures

## Phase 4 — Portfolio presentation

- [x] [Turn the README into an English engineering case study](https://github.com/yngnoise/doomsday/issues/26)
  - Explain the problem, constraints, architecture, and trade-offs
  - Add architecture and event-flow diagrams
  - Link technical claims to tests and measurable results
  - Include demo instructions, screenshots, limitations, and future provider integration

## Delivery order

1. Payment gateway simulator
2. End-to-end coverage for the payment and reservation journey
3. Public demo deployment
4. Responsive and accessible storefront
5. Transactional outbox and background worker
6. Observability and operational health
7. Load and failure testing
8. Live drop and admin UX
9. Portfolio case study
