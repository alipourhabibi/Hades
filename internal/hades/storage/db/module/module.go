// Package module provides PostgreSQL storage for module metadata and ownership lookups.
package module

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// Storage is the domain interface for module persistence.
type Storage interface {
	Create(ctx context.Context, name, ownerId string, visibility registryv1.ModuleVisibility, state registryv1.ModuleState, description, url, defaultLabelName, defaultBranch string) (*registryv1.Module, error)
	ListModules(ctx context.Context, ownerUsername string) ([]*registryv1.Module, error)
	GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registryv1.Module, error)
	GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error)
	CountByOwner(ctx context.Context, ownerID string) (int32, error)
}
