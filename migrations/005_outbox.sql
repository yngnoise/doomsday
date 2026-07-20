BEGIN;

CREATE TABLE IF NOT EXISTS outbox_jobs (
  id              TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
  job_type        TEXT        NOT NULL,
  idempotency_key TEXT        NOT NULL UNIQUE,
  payload         JSONB       NOT NULL,
  status          TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'processing', 'completed', 'dead')),
  attempts        INTEGER     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts    INTEGER     NOT NULL DEFAULT 8 CHECK (max_attempts > 0),
  available_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  locked_at       TIMESTAMPTZ,
  locked_by       TEXT,
  last_error      TEXT,
  completed_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_jobs_available
  ON outbox_jobs(available_at, created_at)
  WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_outbox_jobs_stale
  ON outbox_jobs(locked_at)
  WHERE status = 'processing';

COMMIT;
