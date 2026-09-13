-- Back to `timestamp without time zone`, reading each value as UTC.
--
-- This reinstates the defect in 037: expiry compared against NOW() is wrong by
-- the offset between the application server and the database.

ALTER TABLE api_tokens
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_used_at TYPE TIMESTAMP USING last_used_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMP USING revoked_at AT TIME ZONE 'UTC';

ALTER TABLE audit_log
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC';

ALTER TABLE commits
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMP USING update_time AT TIME ZONE 'UTC';

ALTER TABLE device_grants
    ALTER COLUMN approved_at TYPE TIMESTAMP USING approved_at AT TIME ZONE 'UTC',
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC';

ALTER TABLE email_verifications
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMP USING used_at AT TIME ZONE 'UTC';

ALTER TABLE gitaly_operation_log
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMP USING update_time AT TIME ZONE 'UTC';

ALTER TABLE modules
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMP USING update_time AT TIME ZONE 'UTC';

ALTER TABLE oauth_identities
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC';

ALTER TABLE opa_role_bindings
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

ALTER TABLE password_resets
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMP USING used_at AT TIME ZONE 'UTC';

ALTER TABLE sdk_jobs
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN finished_at TYPE TIMESTAMP USING finished_at AT TIME ZONE 'UTC',
    ALTER COLUMN started_at TYPE TIMESTAMP USING started_at AT TIME ZONE 'UTC';

ALTER TABLE sessions
    ALTER COLUMN absolute_expires_at TYPE TIMESTAMP USING absolute_expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_activity_at TYPE TIMESTAMP USING last_activity_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMP USING revoked_at AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMP USING update_time AT TIME ZONE 'UTC';

ALTER TABLE totp_backup_codes
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMP USING used_at AT TIME ZONE 'UTC';

ALTER TABLE totp_secrets
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN enrolled_at TYPE TIMESTAMP USING enrolled_at AT TIME ZONE 'UTC';

ALTER TABLE users
    ALTER COLUMN create_time TYPE TIMESTAMP USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN email_verified_at TYPE TIMESTAMP USING email_verified_at AT TIME ZONE 'UTC',
    ALTER COLUMN locked_until TYPE TIMESTAMP USING locked_until AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMP USING update_time AT TIME ZONE 'UTC';
