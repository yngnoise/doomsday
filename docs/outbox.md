# Transactional outbox

DOOMSDAY uses PostgreSQL as a durable job queue for side effects that must not
be lost when an API process exits. No separate message broker is required.

## Delivery model

- Business data and its outbox job are committed in the same transaction.
- Workers claim jobs with `FOR UPDATE SKIP LOCKED`, so multiple API instances
  can consume the queue concurrently.
- A processing lease makes jobs recoverable after a worker crash.
- Failed jobs use exponential backoff capped at five minutes.
- A job becomes `dead` after eight failed attempts and retains its last error
  for diagnosis.
- Stable idempotency keys prevent duplicate enqueue operations. Payment events,
  Redis waitlist claims, and email `Message-ID` headers reuse those identities
  during retries.

The delivery guarantee is at least once. Every dispatcher must therefore be
idempotent: a worker may finish a side effect and crash before it records the
job as completed.

## Durable jobs

| Job type | Created with | Side effect |
| --- | --- | --- |
| `payment.simulation` | Payment record | Signed processing and final payment events |
| `email.reservation_confirmation` | Reservation record | Reservation email |
| `email.order_confirmation` | Paid order | Order email |
| `reservation.expiry` | `pending` to `expiring` transition | Redis stock release and SSE update |
| `waitlist.promotion` | Expiry completion | Stable Redis queue claim and promotion email |

Reservation schedulers only claim due rows and enqueue jobs. The claim uses a
row lock and the intermediate `expiring` state, so two schedulers cannot release
the same reservation. Redis stock release remains idempotent across worker
retries.

## Operations

Pending, processing, and dead-letter counts can be inspected directly:

```sql
SELECT status, job_type, COUNT(*)
FROM outbox_jobs
GROUP BY status, job_type
ORDER BY status, job_type;
```

Integration tests cover transaction rollback, duplicate enqueue, retry to dead
letter, stale-lease recovery, duplicate payment delivery, and concurrent
scheduler claims.
