-- Tracks every Gitaly operation (repository creation, commit writes) so that a
-- background cleanup job can compensate (roll back) any operation whose
-- enclosing DB transaction never committed (e.g. after a server crash).
--
-- Lifecycle of a row:
--   1. 'pending'    – row is inserted BEFORE the Gitaly call (auto-committed,
--                    visible immediately so the cleanup job can act on it).
--   2. 'completed'  – updated AFTER the Gitaly call succeeds AND the DB
--                    transaction commits.
--   3. 'failed'     – updated when the Gitaly call itself returns an error
--                    (Gitaly operation never completed).
--   4. 'rolled_back'– updated by the cleanup job (or inline compensation) after
--                    the Gitaly state has been reverted.
--
-- The cleanup job queries:
--   SELECT * FROM gitaly_operation_log
--   WHERE status = 'pending'
--     AND create_time < NOW() - INTERVAL '5 minutes';
-- and calls the appropriate compensating RPC (DeleteRepository / RollbackCommit).

CREATE TABLE IF NOT EXISTS gitaly_operation_log (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    create_time    TIMESTAMP   NOT NULL DEFAULT NOW(),
    update_time    TIMESTAMP   NOT NULL DEFAULT NOW(),
    operation_type VARCHAR(50) NOT NULL,   -- 'create_module', 'commit_files'
    status         VARCHAR(20) NOT NULL DEFAULT 'pending',
    module_name    VARCHAR(255) NOT NULL,  -- Gitaly relative path (owner/module)
    commit_hash    VARCHAR(64),            -- populated on completion for commit_files ops
    user_id        UUID,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_gitaly_op_log_status      ON gitaly_operation_log(status);
CREATE INDEX idx_gitaly_op_log_create_time ON gitaly_operation_log(create_time);
CREATE INDEX idx_gitaly_op_log_module_name ON gitaly_operation_log(module_name);
