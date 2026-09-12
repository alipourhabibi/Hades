-- Reverting to VARCHAR(12) cannot preserve the data: every prefix written by
-- GenerateAPIToken is 15 characters, so a plain type change fails on the first
-- row. The USING clause truncates rather than failing, which is lossy and is
-- the only way this rollback can run at all.
--
-- The loss is confined to display: prefix is shown in token listings so a user
-- can tell their tokens apart, and is never used to authenticate. Truncated
-- prefixes make two tokens harder to distinguish; they do not invalidate them.
ALTER TABLE api_tokens ALTER COLUMN prefix TYPE VARCHAR(12) USING left(prefix, 12);
