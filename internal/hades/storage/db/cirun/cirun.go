// Package cirun provides storage operations for CI run records.  A CI run
// captures the outcome of a lint check and a breaking-change check that are
// executed each time a module commit is pushed.  Records are keyed by
// (module_id, commit_hash) and stored in the ci_runs table (migration 020).
package cirun

import (
	"context"
	"encoding/json"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// CIRunStorage handles CRUD operations for the ci_runs table.
type CIRunStorage struct {
	db querier
}

// New creates a CIRunStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *CIRunStorage {
	return &CIRunStorage{db: pool}
}

// WithTx returns a shallow copy of CIRunStorage that executes queries within
// the given transaction instead of the pool.
func (s *CIRunStorage) WithTx(tx pgx.Tx) *CIRunStorage {
	return &CIRunStorage{db: tx}
}

// GetByModuleAndCommit returns the CI run for the given (moduleID, commitHash)
// pair.  Returns pgx.ErrNoRows if no record exists.
func (s *CIRunStorage) GetByModuleAndCommit(ctx context.Context, moduleID, commitHash string) (*registryv1.CIRun, error) {
	query := `
SELECT id, module_id, commit_hash, lint_passed, breaking_passed,
       COALESCE(lint_errors, '[]'::jsonb),
       COALESCE(breaking_errors, '[]'::jsonb),
       created_at
FROM ci_runs
WHERE module_id = $1 AND commit_hash = $2`

	run := &registryv1.CIRun{}
	var createdAt time.Time
	var lintRaw, breakingRaw []byte

	err := s.db.QueryRow(ctx, query, moduleID, commitHash).Scan(
		&run.Id,
		&run.ModuleId,
		&run.CommitHash,
		&run.LintPassed,
		&run.BreakingPassed,
		&lintRaw,
		&breakingRaw,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	run.CreateTime = timestamppb.New(createdAt)

	if err := json.Unmarshal(lintRaw, &run.LintErrors); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(breakingRaw, &run.BreakingErrors); err != nil {
		return nil, err
	}

	return run, nil
}

// Create inserts a new CI run record, or updates the existing row for the
// same (module_id, commit_hash) pair (upsert semantics).
func (s *CIRunStorage) Create(ctx context.Context, moduleID, commitHash string, lintPassed, breakingPassed bool, lintErrors, breakingErrors []string) (*registryv1.CIRun, error) {
	lintRaw, err := json.Marshal(lintErrors)
	if err != nil {
		return nil, err
	}
	breakingRaw, err := json.Marshal(breakingErrors)
	if err != nil {
		return nil, err
	}

	query := `
INSERT INTO ci_runs (module_id, commit_hash, lint_passed, breaking_passed, lint_errors, breaking_errors)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
ON CONFLICT (module_id, commit_hash) DO UPDATE
  SET lint_passed = EXCLUDED.lint_passed,
      breaking_passed = EXCLUDED.breaking_passed,
      lint_errors = EXCLUDED.lint_errors,
      breaking_errors = EXCLUDED.breaking_errors
RETURNING id, module_id, commit_hash, lint_passed, breaking_passed, lint_errors, breaking_errors, created_at`

	run := &registryv1.CIRun{}
	var createdAt time.Time
	var lintRes, breakingRes []byte

	err = s.db.QueryRow(ctx, query, moduleID, commitHash, lintPassed, breakingPassed, lintRaw, breakingRaw).Scan(
		&run.Id,
		&run.ModuleId,
		&run.CommitHash,
		&run.LintPassed,
		&run.BreakingPassed,
		&lintRes,
		&breakingRes,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	run.CreateTime = timestamppb.New(createdAt)

	if err := json.Unmarshal(lintRes, &run.LintErrors); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(breakingRes, &run.BreakingErrors); err != nil {
		return nil, err
	}

	return run, nil
}
