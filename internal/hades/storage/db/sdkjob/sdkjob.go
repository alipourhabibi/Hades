// Package sdkjob manages SDK code-generation job records. Jobs are enqueued
// by the upload handler (one per generator per commit) and processed by the
// SDK worker. ClaimPending uses SELECT FOR UPDATE SKIP LOCKED so multiple
// workers can run concurrently without contention. Jobs that exceed
// MaxAttempts are marked 'dead' and removed from the work queue permanently.
package sdkjob

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxAttempts is the maximum number of times a job will be retried before
// it is marked 'dead' and removed from the work queue permanently.
const MaxAttempts = 5

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// SDKJob represents a row in the sdk_jobs table.
type SDKJob struct {
	ID             string
	CommitID       string
	ModuleID       string
	Status         string
	Language       string
	Plugin         string
	PluginOptions  string
	OutputLocation string
	ErrorMessage   string
	Attempts       int
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

// SDKJobStorage handles CRUD for sdk_jobs.
type SDKJobStorage struct {
	db querier
}

// New creates an SDKJobStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *SDKJobStorage {
	return &SDKJobStorage{db: pool}
}

// WithTx returns a copy of SDKJobStorage bound to the given transaction.
func (s *SDKJobStorage) WithTx(tx pgx.Tx) *SDKJobStorage {
	return &SDKJobStorage{db: tx}
}

// CreateBatch inserts one sdk_job row per generator in a single batch write.
func (s *SDKJobStorage) CreateBatch(ctx context.Context, commitID, moduleID string, generators []config.GeneratorConfig) error {
	batch := &pgx.Batch{}
	query := `
INSERT INTO sdk_jobs (commit_id, module_id, language, plugin, plugin_options)
VALUES ($1, $2, $3, $4, $5)`
	for _, g := range generators {
		batch.Queue(query, commitID, moduleID, g.Language, g.Plugin, g.Options)
	}
	br := s.db.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range generators {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ClaimPending atomically claims up to limit pending jobs (that have not yet
// exceeded MaxAttempts) by setting their status to 'running'.
// Uses SELECT … FOR UPDATE SKIP LOCKED to avoid contention between workers.
func (s *SDKJobStorage) ClaimPending(ctx context.Context, limit int) ([]*SDKJob, error) {
	query := `
WITH claimed AS (
  SELECT id FROM sdk_jobs
  WHERE status = 'pending'
    AND attempts < $2
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

	rows, err := s.db.Query(ctx, query, limit, MaxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*SDKJob
	for rows.Next() {
		job := &SDKJob{}
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

// MarkSucceeded marks a job as succeeded with the given output location.
func (s *SDKJobStorage) MarkSucceeded(ctx context.Context, jobID, outputLocation string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sdk_jobs SET status='succeeded', output_location=$2, finished_at=NOW() WHERE id=$1`,
		jobID, outputLocation)
	return err
}

// MarkFailed marks a job as failed. If the job has reached MaxAttempts it is
// marked 'dead' (permanently out of the queue); otherwise 'failed' (eligible
// for retry by the next ClaimPending call).
func (s *SDKJobStorage) MarkFailed(ctx context.Context, jobID, errMsg string, attempts int) error {
	status := "failed"
	if attempts >= MaxAttempts {
		status = "dead"
	}
	_, err := s.db.Exec(ctx,
		`UPDATE sdk_jobs SET status=$3, error_message=$2, finished_at=NOW() WHERE id=$1`,
		jobID, errMsg, status)
	return err
}

// ListByModule returns all SDK generation jobs for the given moduleID, ordered
// newest first.  All lifecycle states (pending, running, succeeded, failed,
// dead) are included so callers can surface full job history.
func (s *SDKJobStorage) ListByModule(ctx context.Context, moduleID string) ([]*SDKJob, error) {
	query := `
SELECT id, commit_id, module_id, status, language, plugin,
       plugin_options, COALESCE(output_location,''), COALESCE(error_message,''),
       attempts, created_at, started_at, finished_at
FROM sdk_jobs
WHERE module_id = $1
ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*SDKJob
	for rows.Next() {
		job := &SDKJob{}
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

// ListSucceededByModuleAndLang returns all succeeded SDK jobs for the given
// moduleID and language, ordered newest first.
func (s *SDKJobStorage) ListSucceededByModuleAndLang(ctx context.Context, moduleID, language string) ([]*SDKJob, error) {
	query := `
SELECT id, commit_id, module_id, status, language, plugin,
       plugin_options, COALESCE(output_location,''), COALESCE(error_message,''),
       attempts, created_at, started_at, finished_at
FROM sdk_jobs
WHERE module_id = $1 AND language = $2 AND status = 'succeeded'
ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, moduleID, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*SDKJob
	for rows.Next() {
		job := &SDKJob{}
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

// GetByCommitAndLang returns the most recent succeeded SDK job for the given
// commitID and language. Returns nil, nil if no such job exists.
func (s *SDKJobStorage) GetByCommitAndLang(ctx context.Context, commitID, language string) (*SDKJob, error) {
	query := `
SELECT id, commit_id, module_id, status, language, plugin,
       plugin_options, COALESCE(output_location,''), COALESCE(error_message,''),
       attempts, created_at, started_at, finished_at
FROM sdk_jobs
WHERE commit_id = $1 AND language = $2 AND status = 'succeeded'
ORDER BY created_at DESC
LIMIT 1`

	job := &SDKJob{}
	err := s.db.QueryRow(ctx, query, commitID, language).Scan(
		&job.ID, &job.CommitID, &job.ModuleID,
		&job.Status, &job.Language, &job.Plugin,
		&job.PluginOptions, &job.OutputLocation, &job.ErrorMessage,
		&job.Attempts, &job.CreatedAt, &job.StartedAt, &job.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

// RecoverStaleJobs resets jobs that have been in 'running' state for longer
// than stalenessTimeout back to 'pending', provided they have not yet
// exhausted MaxAttempts. Returns the number of jobs recovered.
//
// This handles the case where a worker process crashed while holding jobs.
func (s *SDKJobStorage) RecoverStaleJobs(ctx context.Context, stalenessTimeout time.Duration) (int64, error) {
	intervalLiteral := fmt.Sprintf("%.0f seconds", stalenessTimeout.Seconds())
	result, err := s.db.Exec(ctx, fmt.Sprintf(`
UPDATE sdk_jobs
SET status = 'pending', started_at = NULL
WHERE status = 'running'
  AND started_at < NOW() - '%s'::interval
  AND attempts < %d`, intervalLiteral, MaxAttempts))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
