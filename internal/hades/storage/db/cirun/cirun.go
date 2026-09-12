// Package cirun provides storage operations for CI run records.
package cirun

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// Storage is the domain interface for CI run persistence.
type Storage interface {
	GetByModuleAndCommit(ctx context.Context, moduleID, commitHash string) (*registryv1.CIRun, error)
	// Create records a run.
	//
	// breakingRan states whether a breaking-change comparison happened at all.
	// breakingPassed alone could not: it was written as true both when the
	// comparison came back clean and when there was nothing to compare, so a
	// skipped check read as a passed one.
	Create(ctx context.Context, run CreateParams) (*registryv1.CIRun, error)
}

// CreateParams describes one CI run to record.
type CreateParams struct {
	ModuleID       string
	CommitHash     string
	LintPassed     bool
	BreakingPassed bool
	BreakingRan    bool
	LintErrors     []string
	BreakingErrors []string
}
