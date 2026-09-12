// Package sdkjob manages SDK code-generation job records.
package sdkjob

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/config"
)

const MaxAttempts = 5

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

// Storage is the domain interface for SDK job persistence.
type Storage interface {
	CreateBatch(ctx context.Context, commitID, moduleID string, generators []config.GeneratorConfig) error
	ClaimPending(ctx context.Context, limit int) ([]*SDKJob, error)
	MarkSucceeded(ctx context.Context, jobID, outputLocation string) error
	MarkFailed(ctx context.Context, jobID, errMsg string, attempts int) error
	// ListByModule returns the module's jobs, newest first, capped at 1000.
	// One row exists per generator per commit, so the set grows with history
	// and the method must not be able to return all of it at once.
	ListByModule(ctx context.Context, moduleID string) ([]*SDKJob, error)
	ListSucceededByModuleAndLang(ctx context.Context, moduleID, language string) ([]*SDKJob, error)
	GetByCommitAndLang(ctx context.Context, commitID, language string) (*SDKJob, error)
	RecoverStaleJobs(ctx context.Context, stalenessTimeout time.Duration) (int64, error)
}
