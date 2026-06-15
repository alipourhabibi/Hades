CREATE TABLE ci_runs (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  module_id        UUID        NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
  commit_hash      TEXT        NOT NULL,
  lint_passed      BOOLEAN     NOT NULL DEFAULT FALSE,
  breaking_passed  BOOLEAN     NOT NULL DEFAULT FALSE,
  lint_errors      JSONB,
  breaking_errors  JSONB,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (module_id, commit_hash)
);

CREATE INDEX ci_runs_module_id_idx ON ci_runs (module_id);
