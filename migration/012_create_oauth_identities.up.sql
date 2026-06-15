CREATE TABLE IF NOT EXISTS oauth_identities (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider     VARCHAR(50)  NOT NULL,
  provider_uid VARCHAR(255) NOT NULL,
  email        VARCHAR(255),
  create_time  TIMESTAMP NOT NULL DEFAULT NOW(),
  UNIQUE(provider, provider_uid)
);
