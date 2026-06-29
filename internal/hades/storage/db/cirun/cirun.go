// Package cirun provides storage operations for CI run records.
package cirun

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// Storage is the domain interface for CI run persistence.
type Storage interface {
	GetByModuleAndCommit(ctx context.Context, moduleID, commitHash string) (*registryv1.CIRun, error)
	Create(ctx context.Context, moduleID, commitHash string, lintPassed, breakingPassed bool, lintErrors, breakingErrors []string) (*registryv1.CIRun, error)
}
