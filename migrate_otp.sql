-- Run: psql -U postgres -d doomsday -f migrate_otp.sql

CREATE TABLE IF NOT EXISTS otp_codes (
  id         TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
  email      TEXT        NOT NULL,
  code       TEXT        NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used       BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_otp_email ON otp_codes(email);

-- Clean up expired codes automatically (requires pg_cron or manual sweep)
-- For now the handler deletes expired codes on each request.
