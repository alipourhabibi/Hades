-- Every timestamp column becomes TIMESTAMPTZ.
--
-- The columns were `timestamp without time zone` but every expiry check
-- compares them against NOW(), which is offset aware. pgx writes a time.Time
-- into a naive column as the client's wall clock, so the stored value carried
-- no zone and NOW() resolved in the server's. With the two apart the comparison
-- was wrong by the difference between them: a session that expired an hour ago
-- read as valid for another three on a UTC+04 server, and one still valid died
-- early to the west.
--
-- It hit every credential, not just sessions: api_tokens, device_grants,
-- email_verifications and password_resets all expire the same way.
--
-- Nothing caught it because the dev compose, the E2E harness and the CI runners
-- all run UTC, where client and server agree and the bug cannot show. It needs
-- an application server on local time, which nothing here forbids.
--
-- ci_runs, notifications and org_memberships were already TIMESTAMPTZ. That
-- split is what "TIMESTAMPTZ vs TIMESTAMP mixed" in the review meant.
--
-- The conversion reads existing values as UTC. That is what a deployment on a
-- UTC server already wrote, and every deployment we ship is one: the compose
-- file, the container and the harness. A server that ran on local time has rows
-- offset by its own zone and needs them corrected by hand, because the zone it
-- used was never recorded.
--
-- PostgreSQL rewrites the column in place. No data migration.

ALTER TABLE api_tokens
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_used_at TYPE TIMESTAMPTZ USING last_used_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMPTZ USING revoked_at AT TIME ZONE 'UTC';

ALTER TABLE audit_log
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC';

ALTER TABLE commits
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMPTZ USING update_time AT TIME ZONE 'UTC';

ALTER TABLE device_grants
    ALTER COLUMN approved_at TYPE TIMESTAMPTZ USING approved_at AT TIME ZONE 'UTC',
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC';

ALTER TABLE email_verifications
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMPTZ USING used_at AT TIME ZONE 'UTC';

ALTER TABLE gitaly_operation_log
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMPTZ USING update_time AT TIME ZONE 'UTC';

ALTER TABLE modules
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMPTZ USING update_time AT TIME ZONE 'UTC';

ALTER TABLE oauth_identities
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC';

ALTER TABLE opa_role_bindings
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

ALTER TABLE password_resets
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMPTZ USING used_at AT TIME ZONE 'UTC';

ALTER TABLE sdk_jobs
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN finished_at TYPE TIMESTAMPTZ USING finished_at AT TIME ZONE 'UTC',
    ALTER COLUMN started_at TYPE TIMESTAMPTZ USING started_at AT TIME ZONE 'UTC';

ALTER TABLE sessions
    ALTER COLUMN absolute_expires_at TYPE TIMESTAMPTZ USING absolute_expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_activity_at TYPE TIMESTAMPTZ USING last_activity_at AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMPTZ USING revoked_at AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMPTZ USING update_time AT TIME ZONE 'UTC';

ALTER TABLE totp_backup_codes
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN used_at TYPE TIMESTAMPTZ USING used_at AT TIME ZONE 'UTC';

ALTER TABLE totp_secrets
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN enrolled_at TYPE TIMESTAMPTZ USING enrolled_at AT TIME ZONE 'UTC';

ALTER TABLE users
    ALTER COLUMN create_time TYPE TIMESTAMPTZ USING create_time AT TIME ZONE 'UTC',
    ALTER COLUMN email_verified_at TYPE TIMESTAMPTZ USING email_verified_at AT TIME ZONE 'UTC',
    ALTER COLUMN locked_until TYPE TIMESTAMPTZ USING locked_until AT TIME ZONE 'UTC',
    ALTER COLUMN update_time TYPE TIMESTAMPTZ USING update_time AT TIME ZONE 'UTC';
