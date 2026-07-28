package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteCIRunStorage implements cirun.Storage using database/sql with SQLite.
type SQLiteCIRunStorage struct {
	db *sql.DB
}

func NewCIRun(db *sql.DB) *SQLiteCIRunStorage {
	return &SQLiteCIRunStorage{db: db}
}

func (s *SQLiteCIRunStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteCIRunStorage) GetByModuleAndCommit(ctx context.Context, moduleID, commitHash string) (*registryv1.CIRun, error) {
	run := &registryv1.CIRun{}
	var createdAt sqltypes.Time
	var lintRaw, breakingRaw []byte
	err := s.q(ctx).QueryRowContext(ctx, `
SELECT id, module_id, commit_hash, lint_passed, breaking_passed,
       COALESCE(lint_errors, '[]'), COALESCE(breaking_errors, '[]'), created_at
FROM ci_runs WHERE module_id = ? AND commit_hash = ?`, moduleID, commitHash).Scan(
		&run.Id, &run.ModuleId, &run.CommitHash,
		&run.LintPassed, &run.BreakingPassed,
		&lintRaw, &breakingRaw, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	run.CreateTime = timestamppb.New(createdAt.V)
	_ = json.Unmarshal(lintRaw, &run.LintErrors)
	_ = json.Unmarshal(breakingRaw, &run.BreakingErrors)
	return run, nil
}

func (s *SQLiteCIRunStorage) Create(ctx context.Context, moduleID, commitHash string, lintPassed, breakingPassed bool, lintErrors, breakingErrors []string) (*registryv1.CIRun, error) {
	lintRaw, _ := json.Marshal(lintErrors)
	breakingRaw, _ := json.Marshal(breakingErrors)
	_, err := s.q(ctx).ExecContext(ctx, `
INSERT INTO ci_runs (module_id, commit_hash, lint_passed, breaking_passed, lint_errors, breaking_errors)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(module_id, commit_hash) DO UPDATE SET
  lint_passed = excluded.lint_passed,
  breaking_passed = excluded.breaking_passed,
  lint_errors = excluded.lint_errors,
  breaking_errors = excluded.breaking_errors`,
		moduleID, commitHash, lintPassed, breakingPassed, lintRaw, breakingRaw)
	if err != nil {
		return nil, err
	}
	return s.GetByModuleAndCommit(ctx, moduleID, commitHash)
}

var _ cirun.Storage = (*SQLiteCIRunStorage)(nil)
