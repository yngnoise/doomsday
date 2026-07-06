-- psql -U postgres -d doomsday

DROP TABLE IF EXISTS reservations;
DROP TABLE IF EXISTS drop_sizes;
DROP TABLE IF EXISTS drops;

CREATE TABLE drops (
  id           TEXT        PRIMARY KEY,
  name         TEXT        NOT NULL,
  description  TEXT        NOT NULL DEFAULT '',
  price_cents  INTEGER     NOT NULL DEFAULT 0,
  total_stock  INTEGER     NOT NULL,
  starts_at    TIMESTAMPTZ NOT NULL,
  ends_at      TIMESTAMPTZ NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Per-size stock. Stock is split evenly on creation.
CREATE TABLE drop_sizes (
  id      TEXT    PRIMARY KEY DEFAULT gen_random_uuid()::text,
  drop_id TEXT    NOT NULL REFERENCES drops(id) ON DELETE CASCADE,
  label   TEXT    NOT NULL,
  stock   INTEGER NOT NULL DEFAULT 0,
  UNIQUE(drop_id, label)
);

CREATE TABLE reservations (
  id         TEXT        PRIMARY KEY,
  drop_id    TEXT        NOT NULL REFERENCES drops(id),
  item_id    TEXT        NOT NULL,
  user_id    TEXT        NOT NULL,
  size       TEXT        NOT NULL DEFAULT '',
  status     TEXT        NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reservations_drop_user ON reservations(drop_id, user_id);
CREATE INDEX idx_reservations_expires   ON reservations(expires_at) WHERE status = 'pending';
CREATE INDEX idx_drop_sizes_drop        ON drop_sizes(drop_id);

-- ── Seed drop: 120 units, 6 sizes × 20 each ──────────────────────────────────
INSERT INTO drops (id, name, description, price_cents, total_stock, starts_at, ends_at)
VALUES (
  'dmsdy-ss25-001',
  'WRAITH FIELD JACKET',
  'Military-grade waxed Ventile. Extracted from the wreckage. Last production run. No restocks. Ever.',
  66600, 120,
  NOW() + INTERVAL '2 minutes',
  NOW() + INTERVAL '32 minutes'
);

INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-ss25-001', 'XS',  20),
  ('dmsdy-ss25-001', 'S',   20),
  ('dmsdy-ss25-001', 'M',   20),
  ('dmsdy-ss25-001', 'L',   20),
  ('dmsdy-ss25-001', 'XL',  20),
  ('dmsdy-ss25-001', 'XXL', 20);

SELECT id, name, total_stock FROM drops;
SELECT label, stock FROM drop_sizes WHERE drop_id = 'dmsdy-ss25-001';
