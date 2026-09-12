package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
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
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	moduleID = sqlutil.ID(moduleID)
	run := &registryv1.CIRun{}
	var createdAt sqltypes.Time
	var lintRaw, breakingRaw []byte
	err := s.q(ctx).QueryRowContext(ctx, `
SELECT id, module_id, commit_hash, lint_passed, breaking_passed, breaking_ran,
       COALESCE(lint_errors, '[]'), COALESCE(breaking_errors, '[]'), created_at
FROM ci_runs WHERE module_id = ? AND commit_hash = ?`, moduleID, commitHash).Scan(
		&run.Id, &run.ModuleId, &run.CommitHash,
		&run.LintPassed, &run.BreakingPassed, &run.BreakingRan,
		&lintRaw, &breakingRaw, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	run.CreateTime = timestamppb.New(createdAt.V)
	// The unmarshal errors are returned, not discarded. Corrupt stored lint
	// output silently became an empty error list, which reads as "the check
	// passed"; PostgreSQL returned the error.
	if err := json.Unmarshal(lintRaw, &run.LintErrors); err != nil {
		return nil, fmt.Errorf("cirun: decode lint_errors: %w", err)
	}
	if err := json.Unmarshal(breakingRaw, &run.BreakingErrors); err != nil {
		return nil, fmt.Errorf("cirun: decode breaking_errors: %w", err)
	}
	run.Id = sqlutil.Canonical(run.Id)
	run.ModuleId = sqlutil.Canonical(run.ModuleId)
	return run, nil
}

func (s *SQLiteCIRunStorage) Create(ctx context.Context, params cirun.CreateParams) (*registryv1.CIRun, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	moduleID := sqlutil.ID(params.ModuleID)
	lintRaw, err := json.Marshal(params.LintErrors)
	if err != nil {
		return nil, fmt.Errorf("cirun: encode lint_errors: %w", err)
	}
	breakingRaw, err := json.Marshal(params.BreakingErrors)
	if err != nil {
		return nil, fmt.Errorf("cirun: encode breaking_errors: %w", err)
	}
	_, err = s.q(ctx).ExecContext(ctx, `
INSERT INTO ci_runs (module_id, commit_hash, lint_passed, breaking_passed, breaking_ran, lint_errors, breaking_errors)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(module_id, commit_hash) DO UPDATE SET
  lint_passed = excluded.lint_passed,
  breaking_passed = excluded.breaking_passed,
  breaking_ran = excluded.breaking_ran,
  lint_errors = excluded.lint_errors,
  breaking_errors = excluded.breaking_errors`,
		moduleID, params.CommitHash, params.LintPassed, params.BreakingPassed,
		params.BreakingRan, lintRaw, breakingRaw)
	if err != nil {
		return nil, err
	}
	return s.GetByModuleAndCommit(ctx, moduleID, params.CommitHash)
}

var _ cirun.Storage = (*SQLiteCIRunStorage)(nil)
