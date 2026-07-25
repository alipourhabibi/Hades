// Package commit provides PostgreSQL storage for commit metadata.
package commit

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
)

// Storage is the domain interface for commit persistence.
type Storage interface {
	Create(ctx context.Context, id uuid.UUID, commitHash, ownerId, moduleId string, digestType registryv1.DigestType, digestValue, createdByUserId, sourceControlUrl string) error
	GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error)
	GetCommitByQuery(ctx context.Context, query map[string]any) (*registryv1.Commit, error)
	GetCommitByOwnerModule(ctx context.Context, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
	ListByModule(ctx context.Context, moduleID string) ([]*registryv1.Commit, error)
	GetByHash(ctx context.Context, commitHash string) (*registryv1.Commit, error)
	GetByHashPrefix(ctx context.Context, prefix string) (*registryv1.Commit, error)
	DeleteByIds(ctx context.Context, ids []string) error
}
