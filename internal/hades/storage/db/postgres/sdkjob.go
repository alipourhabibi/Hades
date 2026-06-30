package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sdkJobBatchQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type sdkJobBatchQuerier interface {
	txkeys.PgxQuerier
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// SDKJobStorage handles CRUD for sdk_jobs.
type SDKJobStorage struct {
	pool *pgxpool.Pool
}

func NewSDKJob(pool *pgxpool.Pool) *SDKJobStorage {
	return &SDKJobStorage{pool: pool}
}

func (s *SDKJobStorage) q(ctx context.Context) sdkJobBatchQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *SDKJobStorage) CreateBatch(ctx context.Context, commitID, moduleID string, generators []config.GeneratorConfig) error {
	batch := &pgx.Batch{}
	query := `
INSERT INTO sdk_jobs (commit_id, module_id, language, plugin, plugin_options)
VALUES ($1, $2, $3, $4, $5)`
	for _, g := range generators {
		batch.Queue(query, commitID, moduleID, g.Language, g.Plugin, g.Options)
	}
	br := s.q(ctx).SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range generators {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *SDKJobStorage) ClaimPending(ctx context.Context, limit int) ([]*sdkjob.SDKJob, error) {
	query := `
WITH claimed AS (
  SELECT id FROM sdk_jobs
  WHERE status = 'pending' AND attempts < $2
  ORDER BY created_at
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
UPDATE sdk_jobs j
SET status = 'running', started_at = NOW(), attempts = attempts + 1
FROM claimed
WHERE j.id = claimed.id
RETURNING j.id, j.commit_id, j.module_id, j.status, j.language, j.plugin,
          j.plugin_options, j.attempts`

	rows, err := s.q(ctx).Query(ctx, query, limit, sdkjob.MaxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*sdkjob.SDKJob
	for rows.Next() {
		job := &sdkjob.SDKJob{}
		if err := rows.Scan(
			&job.ID, &job.CommitID, &job.ModuleID,
			&job.Status, &job.Language, &job.Plugin, &job.PluginOptions,
			&job.Attempts,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *SDKJobStorage) MarkSucceeded(ctx context.Context, jobID, outputLocation string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sdk_jobs SET status='succeeded', output_location=$2, finished_at=NOW() WHERE id=$1`,
		jobID, outputLocation)
	return err
}

func (s *SDKJobStorage) MarkFailed(ctx context.Context, jobID, errMsg string, attempts int) error {
	status := "failed"
	if attempts >= sdkjob.MaxAttempts {
		status = "dead"
	}
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sdk_jobs SET status=$3, error_message=$2, finished_at=NOW() WHERE id=$1`,
		jobID, errMsg, status)
	return err
}

const sdkJobColumns = `id, commit_id, module_id, status, language, plugin,
       plugin_options, COALESCE(output_location,''), COALESCE(error_message,''),
       attempts, created_at, started_at, finished_at`

func scanJobs(rows pgx.Rows) ([]*sdkjob.SDKJob, error) {
	var jobs []*sdkjob.SDKJob
	for rows.Next() {
		job := &sdkjob.SDKJob{}
		if err := rows.Scan(
			&job.ID, &job.CommitID, &job.ModuleID,
			&job.Status, &job.Language, &job.Plugin,
			&job.PluginOptions, &job.OutputLocation, &job.ErrorMessage,
			&job.Attempts, &job.CreatedAt, &job.StartedAt, &job.FinishedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *SDKJobStorage) ListByModule(ctx context.Context, moduleID string) ([]*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT `+sdkJobColumns+` FROM sdk_jobs WHERE module_id = $1 ORDER BY created_at DESC`,
		moduleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *SDKJobStorage) ListSucceededByModuleAndLang(ctx context.Context, moduleID, language string) ([]*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT `+sdkJobColumns+` FROM sdk_jobs WHERE module_id = $1 AND language = $2 AND status = 'succeeded' ORDER BY created_at DESC`,
		moduleID, language,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *SDKJobStorage) GetByCommitAndLang(ctx context.Context, commitID, language string) (*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT `+sdkJobColumns+` FROM sdk_jobs WHERE commit_id = $1 AND language = $2 AND status = 'succeeded' ORDER BY created_at DESC LIMIT 1`,
		commitID, language,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, nil
	}
	return jobs[0], nil
}

func (s *SDKJobStorage) RecoverStaleJobs(ctx context.Context, stalenessTimeout time.Duration) (int64, error) {
	intervalLiteral := fmt.Sprintf("%.0f seconds", stalenessTimeout.Seconds())
	result, err := s.q(ctx).Exec(ctx, fmt.Sprintf(`
UPDATE sdk_jobs
SET status = 'pending', started_at = NULL
WHERE status = 'running'
  AND started_at < NOW() - '%s'::interval
  AND attempts < %d`, intervalLiteral, sdkjob.MaxAttempts))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

var _ sdkjob.Storage = (*SDKJobStorage)(nil)
