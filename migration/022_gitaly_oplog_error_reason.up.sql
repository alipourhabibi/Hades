-- Add an error_reason column to gitaly_operation_log so that failed operations
-- record why they failed, making incident investigation easier.
ALTER TABLE gitaly_operation_log
    ADD COLUMN IF NOT EXISTS error_reason TEXT;
