ALTER TABLE gitaly_operation_log
    DROP COLUMN IF EXISTS error_reason;
