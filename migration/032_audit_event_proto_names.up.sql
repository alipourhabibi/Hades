-- The audit_event enum created in 017 uses lower_snake_case values
-- ('login_success'), while the code inserts event.String() from the generated
-- protobuf enum ('AUDIT_EVENT_TYPE_LOGIN_SUCCESS'). No value matched, so every
-- audit write failed on PostgreSQL with "invalid input value for enum". SQLite
-- stores the column as free TEXT and is the default backend, so the test suite
-- never saw it: production had no audit trail at all.
--
-- The enum is dropped rather than extended. One vocabulary is the point, and
-- the protobuf enum is the one the API speaks; a CHECK constraint keeps the
-- column honest without needing a type change every time a member is added.
-- Members present in the proto but missing from 017 (MODULE_CREATED,
-- MODULE_UPDATED, PASSWORD_RESET) come along for free.

ALTER TABLE audit_log ALTER COLUMN event TYPE TEXT USING event::text;

-- Any rows written before this migration are in the old vocabulary. There
-- should be none, because every insert failed, but a database restored from a
-- dump taken before 017 could have them.
UPDATE audit_log SET event = 'AUDIT_EVENT_TYPE_' || upper(event)
 WHERE event NOT LIKE 'AUDIT_EVENT_TYPE_%';

-- 'password_reset_requested', 'login_new_device' and 'email_changed' existed in
-- the old enum with no proto counterpart. Map the first onto PASSWORD_RESET and
-- park the rest as UNSPECIFIED rather than dropping the rows.
UPDATE audit_log SET event = 'AUDIT_EVENT_TYPE_PASSWORD_RESET'
 WHERE event = 'AUDIT_EVENT_TYPE_PASSWORD_RESET_REQUESTED';
UPDATE audit_log SET event = 'AUDIT_EVENT_TYPE_UNSPECIFIED'
 WHERE event IN ('AUDIT_EVENT_TYPE_LOGIN_NEW_DEVICE', 'AUDIT_EVENT_TYPE_EMAIL_CHANGED');

DROP TYPE IF EXISTS audit_event;

ALTER TABLE audit_log
  ADD CONSTRAINT audit_log_event_check CHECK (event IN (
    'AUDIT_EVENT_TYPE_UNSPECIFIED',
    'AUDIT_EVENT_TYPE_LOGIN_SUCCESS',
    'AUDIT_EVENT_TYPE_LOGIN_FAILED',
    'AUDIT_EVENT_TYPE_ACCOUNT_LOCKED',
    'AUDIT_EVENT_TYPE_EMAIL_VERIFIED',
    'AUDIT_EVENT_TYPE_LOGOUT',
    'AUDIT_EVENT_TYPE_PASSWORD_RESET',
    'AUDIT_EVENT_TYPE_PASSWORD_CHANGED',
    'AUDIT_EVENT_TYPE_API_TOKEN_CREATED',
    'AUDIT_EVENT_TYPE_API_TOKEN_REVOKED',
    'AUDIT_EVENT_TYPE_MODULE_CREATED',
    'AUDIT_EVENT_TYPE_MODULE_UPDATED',
    'AUDIT_EVENT_TYPE_SESSION_REVOKED',
    'AUDIT_EVENT_TYPE_TOTP_ENABLED',
    'AUDIT_EVENT_TYPE_TOTP_DISABLED',
    'AUDIT_EVENT_TYPE_OAUTH_LINKED',
    'AUDIT_EVENT_TYPE_OAUTH_UNLINKED'
  ));
