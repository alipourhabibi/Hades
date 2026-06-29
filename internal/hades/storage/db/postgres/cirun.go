package postgres

import (
	"context"
	"encoding/json"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CIRunStorage handles CRUD operations for the ci_runs table.
type CIRunStorage struct {
	pool *pgxpool.Pool
}

func NewCIRun(pool *pgxpool.Pool) *CIRunStorage {
	return &CIRunStorage{pool: pool}
}

func (s *CIRunStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

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

	err := s.q(ctx).QueryRow(ctx, query, moduleID, commitHash).Scan(
		&run.Id, &run.ModuleId, &run.CommitHash,
		&run.LintPassed, &run.BreakingPassed,
		&lintRaw, &breakingRaw, &createdAt,
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

	err = s.q(ctx).QueryRow(ctx, query, moduleID, commitHash, lintPassed, breakingPassed, lintRaw, breakingRaw).Scan(
		&run.Id, &run.ModuleId, &run.CommitHash,
		&run.LintPassed, &run.BreakingPassed,
		&lintRes, &breakingRes, &createdAt,
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

var _ cirun.Storage = (*CIRunStorage)(nil)
