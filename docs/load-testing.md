# Load, resilience, and reconciliation testing

This suite turns the concurrency and recovery claims into a repeatable experiment. It runs only against the disposable `compose.load.yml` stack; the runner resets and destroys its PostgreSQL and Redis volumes.

## Run the suite

Requirements: Docker Engine with Compose v2, Go 1.25+, and `curl`.

```bash
sh scripts/run-load-tests.sh
```

The command builds the API and a pinned k6 binary with `xk6-sse`, starts PostgreSQL and Redis behind Toxiproxy, and runs every scenario. JSON summaries are written to the ignored `loadtest/results/` directory. The same command runs in GitHub Actions and retains those summaries for 14 days.

Do not point the runner or invariant repair command at shared or production infrastructure. Demo reset truncates the connected application database, and `invariantcheck -repair` overwrites the selected drop's Redis inventory.

## Scenario matrix

| Scenario | Workload | Claim under test |
| --- | --- | --- |
| Drop opening | 25 arrivals/second for 20 seconds, each loading the archive and live drop | Read traffic remains responsive at launch |
| Contention | 180 reservation attempts from 120 virtual users against 120 units | Atomic Redis Lua never produces a server error or negative/oversold stock |
| Checkout | 12 concurrent reservation and asynchronous approved-payment journeys | Each settled payment produces exactly one completed reservation and order |
| SSE | 20 subscribers while 20 users reserve stock | Every subscriber receives a valid non-negative stock event |

The thresholds are deliberately modest enough for a shared GitHub runner: non-streaming HTTP p95 below 1 second, HTTP failure rate below 2%, checkout completion p95 below 15 seconds, and SSE first-event p95 below 5 seconds. The open duration of an SSE stream is not HTTP response latency, so it is measured only by the dedicated first-event trend. Correctness is enforced separately from latency.

## Invariants after every scenario

After each k6 invocation, `go run ./cmd/invariantcheck` compares PostgreSQL with Redis and exits non-zero on any violation:

- `Redis total = drop baseline - pending/expiring/completed reservations`;
- every Redis size counter equals its size baseline minus consumed reservations;
- size counters sum to the total counter and no counter is negative;
- Redis reservation markers exactly match durable consumed reservations;
- completed reservations, orders, and paid/refunded payments have equal counts;
- every order has one settled payment and every settled payment has one order.

The checker returns counts and violation descriptions only; it does not print user or reservation identifiers. During controlled maintenance, `go run ./cmd/invariantcheck -repair` can rebuild Redis counters and markers from PostgreSQL before performing the same check. Writes for that drop must be quiesced while repair runs.

## Dependency failure experiment

The API connects to PostgreSQL and Redis through Toxiproxy. The runner disables each proxy independently and proves that:

1. readiness and dependency health return `503`;
2. liveness remains `200`, so an orchestrator does not restart a healthy process merely because a dependency is unavailable;
3. dependency health identifies the failed component without exposing connection details;
4. readiness returns to `200` when connectivity is restored;
5. the final PostgreSQL/Redis invariant check still passes.

## Reference baseline

The first CI baseline was recorded on 21 July 2026 by GitHub Actions `ubuntu-24.04` in [workflow run 29828706918](https://github.com/yngnoise/doomsday/actions/runs/29828706918). Values are taken from its k6 JSON summary artifacts, not estimated from application logs. All checks, both dependency recovery experiments, and every post-scenario invariant check passed.

| Scenario | Requests / flows | HTTP p95 | Error rate | Scenario-specific result |
| --- | ---: | ---: | ---: | --- |
| Drop opening | 1,004 requests | 2.12 ms | 0% | 501 iterations at 25 arrivals/second |
| Contention | 180 attempts | 115.26 ms | 0% | All outcomes safe; no invariant violations |
| Checkout | 12 completed flows | 22.27 ms | 0% | Completion p95 5.56 s; 12/12 paid |
| SSE | 20 subscribers + 20 publishers | 33.66 ms non-streaming | 0% | 20/20 received an event; first-event p95 2.03 s |

Hosted CI numbers are a regression baseline for this repository, not a capacity promise. Re-run the suite on production-like infrastructure before making sizing or SLO decisions.
