ALTER TABLE users
  DROP COLUMN IF EXISTS email_verified_at,
  DROP COLUMN IF EXISTS failed_login_count,
  DROP COLUMN IF EXISTS locked_until;
