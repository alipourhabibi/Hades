DO $$ BEGIN
  CREATE TYPE audit_event AS ENUM (
    'login_success', 'login_failed', 'login_new_device',
    'logout', 'password_changed', 'password_reset_requested',
    'email_changed', 'email_verified',
    'totp_enabled', 'totp_disabled',
    'api_token_created', 'api_token_revoked',
    'session_revoked', 'account_locked',
    'oauth_linked', 'oauth_unlinked'
  );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS audit_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
  event       audit_event NOT NULL,
  ip_address  VARCHAR(45),
  user_agent  TEXT,
  metadata    JSONB,
  create_time TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_user_id     ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_create_time ON audit_log(create_time DESC);
