-- See migration/035: the highest TOTP time step accepted for an account.
ALTER TABLE totp_secrets ADD COLUMN last_used_counter INTEGER;
