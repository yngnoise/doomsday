-- ── 5 new drops ──────────────────────────────────────────────────────────────
-- Run: psql -U postgres -d doomsday -f seed_drops.sql

INSERT INTO drops (id, name, description, price_cents, total_stock, starts_at, ends_at) VALUES

('dmsdy-ss25-002',
 'OBSIDIAN CARGO PANT',
 'Heavy-gauge ripstop. Articulated knees. 9 pockets. Built for terrain that has no name.',
 44400, 80,
 NOW() + INTERVAL '1 day 4 hours',
 NOW() + INTERVAL '1 day 4 hours 30 minutes'),

('dmsdy-ss25-003',
 'VOID TECH HOODIE',
 'Bonded fleece shell. Sealed seams. Drop shoulder. The last hoodie you will ever need.',
 38800, 60,
 NOW() + INTERVAL '3 days',
 NOW() + INTERVAL '3 days 30 minutes'),

('dmsdy-ss25-004',
 'PHANTOM OVERSHIRT',
 'Waxed canvas. Chest map pocket. Detachable hood. Issued. Not sold. Until now.',
 52200, 50,
 NOW() - INTERVAL '2 days',
 NOW() - INTERVAL '2 days' + INTERVAL '30 minutes'),

('dmsdy-ss25-005',
 'BLACKOUT FIELD VEST',
 'D30 impact panels. MOLLE webbing. 14 pockets. Tactical utility. Zero compromise.',
 61100, 40,
 NOW() - INTERVAL '5 days',
 NOW() - INTERVAL '5 days' + INTERVAL '30 minutes'),

('dmsdy-fw25-001',
 'WRAITH LINER JACKET',
 'Primaloft Gold insulation. Featherweight shell. -20°C rated. The last layer.',
 77700, 100,
 NOW() + INTERVAL '7 days',
 NOW() + INTERVAL '7 days 30 minutes');

-- ── Sizes — 6 labels, stock split evenly ─────────────────────────────────────

-- OBSIDIAN CARGO PANT — 80 units / 6 sizes = 13 each + 2 remainder to XXL
INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-ss25-002', 'XS', 13), ('dmsdy-ss25-002', 'S',  13),
  ('dmsdy-ss25-002', 'M',  13), ('dmsdy-ss25-002', 'L',  13),
  ('dmsdy-ss25-002', 'XL', 13), ('dmsdy-ss25-002', 'XXL',15);

-- VOID TECH HOODIE — 60 units / 6 sizes = 10 each
INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-ss25-003', 'XS', 10), ('dmsdy-ss25-003', 'S',  10),
  ('dmsdy-ss25-003', 'M',  10), ('dmsdy-ss25-003', 'L',  10),
  ('dmsdy-ss25-003', 'XL', 10), ('dmsdy-ss25-003', 'XXL',10);

-- PHANTOM OVERSHIRT — 50 units / 6 sizes = 8 each + 2 remainder to XXL
INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-ss25-004', 'XS', 8), ('dmsdy-ss25-004', 'S',  8),
  ('dmsdy-ss25-004', 'M',  8), ('dmsdy-ss25-004', 'L',  8),
  ('dmsdy-ss25-004', 'XL', 8), ('dmsdy-ss25-004', 'XXL',10);

-- BLACKOUT FIELD VEST — 40 units / 6 sizes = 6 each + 4 remainder to XXL
INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-ss25-005', 'XS', 6), ('dmsdy-ss25-005', 'S',  6),
  ('dmsdy-ss25-005', 'M',  6), ('dmsdy-ss25-005', 'L',  6),
  ('dmsdy-ss25-005', 'XL', 6), ('dmsdy-ss25-005', 'XXL',10);

-- WRAITH LINER JACKET — 100 units / 6 sizes = 16 each + 4 remainder to XXL
INSERT INTO drop_sizes (drop_id, label, stock) VALUES
  ('dmsdy-fw25-001', 'XS', 16), ('dmsdy-fw25-001', 'S',  16),
  ('dmsdy-fw25-001', 'M',  16), ('dmsdy-fw25-001', 'L',  16),
  ('dmsdy-fw25-001', 'XL', 16), ('dmsdy-fw25-001', 'XXL',20);

-- ── Verify ────────────────────────────────────────────────────────────────────
SELECT
  d.id,
  d.name,
  d.total_stock,
  to_char(d.starts_at, 'DD Mon HH24:MI') AS starts,
  CASE
    WHEN NOW() < d.starts_at THEN 'UPCOMING'
    WHEN NOW() > d.ends_at   THEN 'ENDED'
    ELSE 'LIVE'
  END AS phase
FROM drops d
ORDER BY d.starts_at DESC;
