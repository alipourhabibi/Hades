-- Record the highest TOTP time step accepted for an account.
--
-- Validation runs with a skew of one step either side, so a code is live for
-- roughly ninety seconds. Nothing recorded which step had been consumed, so a
-- code observed in transit, in a debug log, or over the user's shoulder could
-- be replayed for the rest of that window. ConsumeCounter refuses any step at
-- or below this value, which makes a code single-use.
ALTER TABLE totp_secrets ADD COLUMN IF NOT EXISTS last_used_counter BIGINT;
