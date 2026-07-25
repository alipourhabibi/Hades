package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// SQLiteSDKJobStorage implements sdkjob.Storage using database/sql with SQLite.
type SQLiteSDKJobStorage struct {
	db *sql.DB
}

func NewSDKJob(db *sql.DB) *SQLiteSDKJobStorage {
	return &SQLiteSDKJobStorage{db: db}
}

func (s *SQLiteSDKJobStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteSDKJobStorage) CreateBatch(ctx context.Context, commitID, moduleID string, generators []config.GeneratorConfig) error {
	for _, g := range generators {
		_, err := s.q(ctx).ExecContext(ctx,
			`INSERT INTO sdk_jobs (commit_id, module_id, language, plugin, plugin_options) VALUES (?, ?, ?, ?, ?)`,
			commitID, moduleID, g.Language, g.Plugin, g.Options)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteSDKJobStorage) ClaimPending(ctx context.Context, limit int) ([]*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT id FROM sdk_jobs
WHERE status = 'pending' AND attempts < ?
ORDER BY created_at
LIMIT ?`, sdkjob.MaxAttempts, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()

	var jobs []*sdkjob.SDKJob
	for _, id := range ids {
		_, err := s.q(ctx).ExecContext(ctx,
			`UPDATE sdk_jobs SET status = 'running', started_at = datetime('now'), attempts = attempts + 1 WHERE id = ?`, id)
		if err != nil {
			return nil, err
		}
		job := &sdkjob.SDKJob{}
		if err := s.q(ctx).QueryRowContext(ctx,
			`SELECT id, commit_id, module_id, status, language, plugin, plugin_options, attempts FROM sdk_jobs WHERE id = ?`, id,
		).Scan(&job.ID, &job.CommitID, &job.ModuleID, &job.Status, &job.Language, &job.Plugin, &job.PluginOptions, &job.Attempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *SQLiteSDKJobStorage) MarkSucceeded(ctx context.Context, jobID, outputLocation string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE sdk_jobs SET status='succeeded', output_location=?, finished_at=datetime('now') WHERE id=?`,
		outputLocation, jobID)
	return err
}

func (s *SQLiteSDKJobStorage) MarkFailed(ctx context.Context, jobID, errMsg string, attempts int) error {
	status := "failed"
	if attempts >= sdkjob.MaxAttempts {
		status = "dead"
	}
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE sdk_jobs SET status=?, error_message=?, finished_at=datetime('now') WHERE id=?`,
		status, errMsg, jobID)
	return err
}

const sqliteSDKJobCols = `id, commit_id, module_id, status, language, plugin,
       plugin_options, COALESCE(output_location,''), COALESCE(error_message,''),
       attempts, created_at, started_at, finished_at`

func scanSQLiteJobs(rows *sql.Rows) ([]*sdkjob.SDKJob, error) {
	var jobs []*sdkjob.SDKJob
	for rows.Next() {
		job := &sdkjob.SDKJob{}
		var createdAt sqltypes.Time
		var startedAt, finishedAt sqltypes.NullTime
		if err := rows.Scan(
			&job.ID, &job.CommitID, &job.ModuleID,
			&job.Status, &job.Language, &job.Plugin,
			&job.PluginOptions, &job.OutputLocation, &job.ErrorMessage,
			&job.Attempts, &createdAt, &startedAt, &finishedAt,
		); err != nil {
			return nil, err
		}
		job.CreatedAt = createdAt.V
		job.StartedAt = startedAt.Ptr()
		job.FinishedAt = finishedAt.Ptr()
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *SQLiteSDKJobStorage) ListByModule(ctx context.Context, moduleID string) ([]*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteSDKJobCols+` FROM sdk_jobs WHERE module_id = ? ORDER BY created_at DESC`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteJobs(rows)
}

func (s *SQLiteSDKJobStorage) ListSucceededByModuleAndLang(ctx context.Context, moduleID, language string) ([]*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteSDKJobCols+` FROM sdk_jobs WHERE module_id = ? AND language = ? AND status = 'succeeded' ORDER BY created_at DESC`,
		moduleID, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteJobs(rows)
}

func (s *SQLiteSDKJobStorage) GetByCommitAndLang(ctx context.Context, commitID, language string) (*sdkjob.SDKJob, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteSDKJobCols+` FROM sdk_jobs WHERE commit_id = ? AND language = ? AND status = 'succeeded' ORDER BY created_at DESC LIMIT 1`,
		commitID, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs, err := scanSQLiteJobs(rows)
	if err != nil || len(jobs) == 0 {
		return nil, err
	}
	return jobs[0], nil
}

func (s *SQLiteSDKJobStorage) RecoverStaleJobs(ctx context.Context, stalenessTimeout time.Duration) (int64, error) {
	threshold := time.Now().Add(-stalenessTimeout).Format(time.RFC3339)
	result, err := s.q(ctx).ExecContext(ctx, fmt.Sprintf(`
UPDATE sdk_jobs
SET status = 'pending', started_at = NULL
WHERE status = 'running'
  AND started_at < '%s'
  AND attempts < %d`, threshold, sdkjob.MaxAttempts))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var _ sdkjob.Storage = (*SQLiteSDKJobStorage)(nil)
