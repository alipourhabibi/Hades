-- Remove composite unique index
DROP INDEX IF EXISTS idx_commits_module_commit_digest_unique;

-- Restore previous unique indexes
CREATE UNIQUE INDEX idx_commits_commit_hash_unique
ON commits(commit_hash);

CREATE UNIQUE INDEX idx_commits_digest_value_unique
ON commits(digest_value);

-- Remove partial unique index
DROP INDEX IF EXISTS idx_modules_url_unique_non_empty;

-- Restore original uniqueness
ALTER TABLE modules
ADD CONSTRAINT modules_url_key UNIQUE (url);
