BEGIN;

CREATE TABLE IF NOT EXISTS payments (
  id             TEXT        PRIMARY KEY,
  reservation_id TEXT        NOT NULL REFERENCES reservations(id),
  user_id        TEXT        NOT NULL,
  amount_cents   INTEGER     NOT NULL CHECK (amount_cents >= 0),
  currency       TEXT        NOT NULL DEFAULT 'USD',
  scenario       TEXT        NOT NULL CHECK (scenario IN ('success', 'declined', 'cancelled', 'timeout')),
  status         TEXT        NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending', 'processing', 'paid', 'failed', 'refunded')),
  failure_code   TEXT,
  email          TEXT        NOT NULL,
  customer_name  TEXT        NOT NULL,
  address        TEXT        NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_user_created
  ON payments(user_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_active_reservation
  ON payments(reservation_id)
  WHERE status IN ('pending', 'processing', 'paid', 'refunded');

CREATE TABLE IF NOT EXISTS payment_events (
  id           TEXT        PRIMARY KEY,
  payment_id   TEXT        NOT NULL REFERENCES payments(id),
  event_type   TEXT        NOT NULL,
  payload      JSONB       NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_events_payment_created
  ON payment_events(payment_id, created_at DESC);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_id TEXT REFERENCES payments(id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_payment_unique
  ON orders(payment_id)
  WHERE payment_id IS NOT NULL;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
  CHECK (status IN ('completed', 'refunded'));

COMMIT;
