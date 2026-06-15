-- Remove incorrect unique indexes
DROP INDEX IF EXISTS idx_commits_commit_hash_unique;
DROP INDEX IF EXISTS idx_commits_digest_value_unique;

-- Create proper composite unique index
CREATE UNIQUE INDEX idx_commits_module_commit_digest_unique
ON commits(module_id, commit_hash, digest_value);

ALTER TABLE modules DROP CONSTRAINT IF EXISTS modules_url_key;
DROP INDEX IF EXISTS modules_url_key;
DROP INDEX IF EXISTS idx_modules_url_unique;

-- Create partial unique index
CREATE UNIQUE INDEX idx_modules_url_unique_non_empty
ON modules(url)
WHERE url IS NOT NULL AND url <> '';
