// Package gitaly provides the Gitaly gRPC storage layer. Each sub-service
// (blob, commit, diff, operation, repository, tree) wraps a Gitaly gRPC
// client and exposes domain-specific operations.
package gitaly

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/diff"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/tree"
)

// StorageService aggregates all Gitaly sub-service clients.
type StorageService struct {
	CommitService     *commit.CommitService
	BlobService       *blob.BlobService
	OperattionService *operation.OperationService
	RepositoryService *repository.RepositoryService
	DiffService       *diff.DiffService
	TreeService       *tree.TreeService
}

// NewStorage dials the Gitaly server and initialises all sub-service clients.
func NewStorage(c config.Gitaly) (*StorageService, error) {
	commitService, err := commit.NewDefault(c)
	if err != nil {
		return nil, err
	}

	operationService, err := operation.NewDefault(c)
	if err != nil {
		return nil, err
	}

	blobService, err := blob.NewDefault(c)
	if err != nil {
		return nil, err
	}

	repositoryService, err := repository.NewDefault(c)
	if err != nil {
		return nil, err
	}

	diffService, err := diff.NewDefault(c)
	if err != nil {
		return nil, err
	}

	treeService, err := tree.NewDefault(c)
	if err != nil {
		return nil, err
	}

	return &StorageService{
		CommitService:     commitService,
		OperattionService: operationService,
		RepositoryService: repositoryService,
		BlobService:       blobService,
		DiffService:       diffService,
		TreeService:       treeService,
	}, nil
}
