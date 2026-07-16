BEGIN;

CREATE TABLE IF NOT EXISTS orders (
  id             TEXT        PRIMARY KEY,
  reservation_id TEXT        NOT NULL UNIQUE REFERENCES reservations(id),
  drop_id        TEXT        NOT NULL REFERENCES drops(id),
  item_id        TEXT        NOT NULL,
  user_id        TEXT        NOT NULL,
  size           TEXT        NOT NULL,
  email          TEXT        NOT NULL,
  customer_name  TEXT        NOT NULL,
  address        TEXT        NOT NULL,
  amount_cents   INTEGER     NOT NULL CHECK (amount_cents >= 0),
  status         TEXT        NOT NULL DEFAULT 'completed' CHECK (status IN ('completed')),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_drop_created
  ON orders(drop_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_user_created
  ON orders(user_id, created_at DESC);

COMMIT;
