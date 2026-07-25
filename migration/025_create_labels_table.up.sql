CREATE TABLE IF NOT EXISTS labels (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id UUID NOT NULL,
    name      TEXT NOT NULL,
    commit_id UUID,

    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY (commit_id) REFERENCES commits(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_module_name ON labels(module_id, name);
