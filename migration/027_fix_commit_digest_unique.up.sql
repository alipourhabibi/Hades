-- digest_value must be unique per module, not globally.
-- Two different modules can have the same digest if they contain identical files.
DROP INDEX IF EXISTS idx_commits_digest_value_unique;
CREATE UNIQUE INDEX idx_commits_module_digest_unique ON commits(module_id, digest_value);
