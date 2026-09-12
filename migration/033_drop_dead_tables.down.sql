-- Recreates the tables 033 dropped, as 005 and 025 defined them. Nothing reads
-- them; this exists so the migration is reversible.
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    SERIAL PRIMARY KEY,
    ptype VARCHAR(100),
    v0    VARCHAR(100),
    v1    VARCHAR(100),
    v2    VARCHAR(100),
    v3    VARCHAR(100),
    v4    VARCHAR(100),
    v5    VARCHAR(100)
);

CREATE TABLE IF NOT EXISTS labels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id  UUID NOT NULL,
    name       VARCHAR(255) NOT NULL,
    commit_id  UUID,
    FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY (commit_id) REFERENCES commits(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_module_name ON labels(module_id, name);
