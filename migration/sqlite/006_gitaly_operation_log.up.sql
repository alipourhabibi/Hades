-- The git operation log, which existed only on PostgreSQL.
--
-- Callers nil-guarded it, so on the default backend the crash-compensation log
-- was silently disabled: a git write orphaned by a crash between the git call
-- and the database commit was never reconciled, and nothing in the logs or the
-- documentation said so.

CREATE TABLE IF NOT EXISTS gitaly_operation_log (
    id             TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time    DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time    DATETIME NOT NULL DEFAULT (datetime('now')),
    operation_type TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    module_name    TEXT NOT NULL,
    commit_hash    TEXT,
    user_id        TEXT REFERENCES users(id) ON DELETE SET NULL,
    error_reason   TEXT
);

CREATE INDEX IF NOT EXISTS idx_gitaly_op_log_status      ON gitaly_operation_log(status);
CREATE INDEX IF NOT EXISTS idx_gitaly_op_log_create_time ON gitaly_operation_log(create_time);
CREATE INDEX IF NOT EXISTS idx_gitaly_op_log_module_name ON gitaly_operation_log(module_name);
