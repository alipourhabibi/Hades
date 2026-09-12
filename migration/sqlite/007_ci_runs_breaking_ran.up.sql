-- See migration/034: whether the breaking-change comparison actually ran.
ALTER TABLE ci_runs ADD COLUMN breaking_ran INTEGER NOT NULL DEFAULT 0;
