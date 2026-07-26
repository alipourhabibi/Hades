-- Composite index for DISTINCT ON (module_id) ORDER BY module_id, create_time DESC
-- used by GetCommitByOwnerModule to fetch the latest commit per module.
CREATE INDEX IF NOT EXISTS idx_commits_module_id_create_time
    ON commits (module_id, create_time DESC);
