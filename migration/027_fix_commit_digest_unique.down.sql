DROP INDEX IF EXISTS idx_commits_module_digest_unique;
-- IF NOT EXISTS: 023's down migration recreates this same index, and a rollback
-- runs this one first. See the note there.
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_digest_value_unique ON commits(digest_value);
