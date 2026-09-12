-- Remove composite unique index
DROP INDEX IF EXISTS idx_commits_module_commit_digest_unique;

-- Restore previous unique indexes.
--
-- IF NOT EXISTS on both, because 027's down migration recreates
-- idx_commits_digest_value_unique too and runs first. Without the guard,
-- "migrate down -all" from head failed here with "relation
-- idx_commits_digest_value_unique already exists" and stopped partway, so the
-- documented rollback could not complete on any database that had reached 027.
-- The up migrations in this chain all use DROP INDEX IF EXISTS; the asymmetry
-- was the defect.
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_commit_hash_unique
ON commits(commit_hash);

CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_digest_value_unique
ON commits(digest_value);

-- Remove partial unique index
DROP INDEX IF EXISTS idx_modules_url_unique_non_empty;

-- Restore original uniqueness. PostgreSQL has no IF NOT EXISTS for ADD
-- CONSTRAINT, so it is dropped first: repeating the constraint is the same
-- hazard as repeating the index.
ALTER TABLE modules DROP CONSTRAINT IF EXISTS modules_url_key;
ALTER TABLE modules ADD CONSTRAINT modules_url_key UNIQUE (url);
