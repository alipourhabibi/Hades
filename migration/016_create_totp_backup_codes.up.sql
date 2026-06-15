CREATE TABLE IF NOT EXISTS totp_backup_codes (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash   VARCHAR(64) NOT NULL,
  used_at     TIMESTAMP,
  create_time TIMESTAMP NOT NULL DEFAULT NOW()
);
