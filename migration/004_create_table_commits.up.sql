-- Create commits table
CREATE TABLE IF NOT EXISTS commits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commit_hash VARCHAR(40) NOT NULL,
    create_time TIMESTAMP NOT NULL DEFAULT NOW(),
    update_time TIMESTAMP NOT NULL DEFAULT NOW(),
    owner_id UUID NOT NULL,
    module_id UUID NOT NULL,
    digest_type SMALLINT NOT NULL,
    digest_value VARCHAR(128) NOT NULL,
    created_by_user_id UUID,
    source_control_url TEXT,

    -- Foreign key constraints
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes for performance
CREATE INDEX idx_commits_owner_id ON commits(owner_id);
CREATE INDEX idx_commits_module_id ON commits(module_id);
CREATE INDEX idx_commits_created_by_user_id ON commits(created_by_user_id);
CREATE INDEX idx_commits_commit_hash ON commits(commit_hash);

-- Ensure commit_hash is unique (optional, based on your comment)
CREATE UNIQUE INDEX idx_commits_commit_hash_unique ON commits(commit_hash);

-- Ensure digest_value is unique (optional, based on your comment)
CREATE UNIQUE INDEX idx_commits_digest_value_unique ON commits(digest_value);
