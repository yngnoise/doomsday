BEGIN;

CREATE TABLE IF NOT EXISTS drops (
  id           TEXT        PRIMARY KEY,
  name         TEXT        NOT NULL,
  description  TEXT        NOT NULL DEFAULT '',
  price_cents  INTEGER     NOT NULL DEFAULT 0,
  total_stock  INTEGER     NOT NULL,
  starts_at    TIMESTAMPTZ NOT NULL,
  ends_at      TIMESTAMPTZ NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS drop_sizes (
  id      TEXT    PRIMARY KEY DEFAULT gen_random_uuid()::text,
  drop_id TEXT    NOT NULL REFERENCES drops(id) ON DELETE CASCADE,
  label   TEXT    NOT NULL,
  stock   INTEGER NOT NULL DEFAULT 0,
  UNIQUE(drop_id, label)
);

CREATE TABLE IF NOT EXISTS reservations (
  id         TEXT        PRIMARY KEY,
  drop_id    TEXT        NOT NULL REFERENCES drops(id),
  item_id    TEXT        NOT NULL,
  user_id    TEXT        NOT NULL,
  size       TEXT        NOT NULL DEFAULT '',
  status     TEXT        NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reservations_drop_user
  ON reservations(drop_id, user_id);

CREATE INDEX IF NOT EXISTS idx_reservations_expires
  ON reservations(expires_at)
  WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_drop_sizes_drop
  ON drop_sizes(drop_id);

COMMIT;
