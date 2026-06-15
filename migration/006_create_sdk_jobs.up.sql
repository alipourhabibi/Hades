CREATE TABLE sdk_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commit_id       UUID NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
    module_id       UUID NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    language        VARCHAR(50)  NOT NULL,
    plugin          VARCHAR(100) NOT NULL,
    plugin_options  TEXT         NOT NULL DEFAULT '',
    output_location TEXT,
    error_message   TEXT,
    attempts        INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMP,
    finished_at     TIMESTAMP
);

CREATE INDEX idx_sdk_jobs_status     ON sdk_jobs(status);
CREATE INDEX idx_sdk_jobs_commit_id  ON sdk_jobs(commit_id);
