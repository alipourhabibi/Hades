CREATE TABLE IF NOT EXISTS totp_secrets (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  secret_enc  TEXT NOT NULL,
  enabled     BOOLEAN NOT NULL DEFAULT FALSE,
  enrolled_at TIMESTAMP,
  create_time TIMESTAMP NOT NULL DEFAULT NOW()
);
