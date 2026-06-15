CREATE TABLE IF NOT EXISTS api_tokens (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         VARCHAR(255) NOT NULL,
  prefix       VARCHAR(12)  NOT NULL,
  token_hash   VARCHAR(64)  UNIQUE NOT NULL,
  scopes       TEXT[],
  expires_at   TIMESTAMP,
  last_used_at TIMESTAMP,
  revoked_at   TIMESTAMP,
  create_time  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
