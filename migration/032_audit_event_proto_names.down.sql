-- Reverses 032. The enum is recreated exactly as 017 defined it, so rolling
-- back also restores the defect 032 fixed: every audit write will fail again.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_event_check;

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

-- Rows whose event has no counterpart in the old enum would fail the cast, so
-- they are removed first.
DELETE FROM audit_log
 WHERE lower(replace(event, 'AUDIT_EVENT_TYPE_', '')) NOT IN (
   'login_success', 'login_failed', 'login_new_device',
   'logout', 'password_changed', 'password_reset_requested',
   'email_changed', 'email_verified',
   'totp_enabled', 'totp_disabled',
   'api_token_created', 'api_token_revoked',
   'session_revoked', 'account_locked',
   'oauth_linked', 'oauth_unlinked'
 );

ALTER TABLE audit_log
  ALTER COLUMN event TYPE audit_event
  USING lower(replace(event, 'AUDIT_EVENT_TYPE_', ''))::audit_event;
