CREATE TABLE IF NOT EXISTS device_grants (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  device_code_hash VARCHAR(64) UNIQUE NOT NULL,
  user_code        VARCHAR(10) UNIQUE NOT NULL,
  user_id          UUID REFERENCES users(id) ON DELETE CASCADE,
  api_token_id     UUID REFERENCES api_tokens(id) ON DELETE SET NULL,
  approved_at      TIMESTAMP,
  expires_at       TIMESTAMP NOT NULL,
  create_time      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_device_grants_user_code ON device_grants(user_code);
