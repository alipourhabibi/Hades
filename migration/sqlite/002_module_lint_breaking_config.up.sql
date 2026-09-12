-- Per-module lint preset and breaking-change enforcement.
--
-- Kept as a separate migration rather than folded into 001 so that databases
-- created before these columns existed reach the same schema as a fresh one.

ALTER TABLE modules ADD COLUMN lint_preset INTEGER NOT NULL DEFAULT 1;
ALTER TABLE modules ADD COLUMN breaking_enabled INTEGER NOT NULL DEFAULT 1;
