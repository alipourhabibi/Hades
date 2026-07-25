DROP INDEX IF EXISTS idx_commits_module_digest_unique;
CREATE UNIQUE INDEX idx_commits_digest_value_unique ON commits(digest_value);
