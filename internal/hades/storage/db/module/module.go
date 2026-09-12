// Package module provides PostgreSQL storage for module metadata and ownership lookups.
package module

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// Storage is the domain interface for module persistence.
type Storage interface {
	Create(ctx context.Context, name, ownerId string, visibility registryv1.ModuleVisibility, state registryv1.ModuleState, description, url, defaultLabelName, defaultBranch string, lintPreset registryv1.LintPreset, breakingEnabled bool) (*registryv1.Module, error)
	Update(ctx context.Context, req *registryv1.UpdateModuleRequest) (*registryv1.Module, error)
	ListModules(ctx context.Context, ownerUsername string, limit, offset int) ([]*registryv1.Module, error)
	// ListVisibleModules is ListModules narrowed to what a caller can plausibly
	// read: public modules, modules they own, and modules covered by one of
	// their OPA role bindings.
	//
	// It is a pre-filter, not the authorization decision. The OPA policy remains
	// the authority and callers must still run CheckReadAccess on the result;
	// this exists so a page is not mostly discarded after the query, which made
	// pagination return near-empty pages and cost one policy evaluation per row
	// scanned.
	//
	// An empty subject means an anonymous caller, which restricts the result to
	// public modules.
	ListVisibleModules(ctx context.Context, ownerUsername, subject, subjectID string, limit, offset int) ([]*registryv1.Module, error)
	GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registryv1.Module, error)
	GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error)
	CountByOwner(ctx context.Context, ownerID string) (int32, error)
}
