BEGIN;

CREATE TABLE IF NOT EXISTS users (
  id         TEXT        PRIMARY KEY,
  email      TEXT        NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS otp_codes (
  id         TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
  email      TEXT        NOT NULL,
  code_hash  TEXT        NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used       BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Existing plaintext OTP values are deliberately invalidated during migration.
ALTER TABLE otp_codes ADD COLUMN IF NOT EXISTS code_hash TEXT;
DELETE FROM otp_codes WHERE code_hash IS NULL;
ALTER TABLE otp_codes ALTER COLUMN code_hash SET NOT NULL;
ALTER TABLE otp_codes DROP COLUMN IF EXISTS code;

CREATE INDEX IF NOT EXISTS idx_otp_email_active
  ON otp_codes(email, created_at DESC)
  WHERE used = FALSE;

COMMIT;
