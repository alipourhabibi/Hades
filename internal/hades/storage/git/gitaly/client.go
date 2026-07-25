package gitaly

import (
	"github.com/alipourhabibi/Hades/config"
)

// StorageService aggregates all Gitaly gRPC sub-service clients.
type StorageService struct {
	CommitService     *CommitService
	BlobService       *BlobService
	OperattionService *OperationService
	RepositoryService *RepositoryService
	DiffService       *DiffService
	TreeService       *TreeService
}

// NewStorage dials the Gitaly server and initialises all sub-service clients.
func NewStorage(c config.Gitaly) (*StorageService, error) {
	commitService, err := newCommitService(c)
	if err != nil {
		return nil, err
	}

	operationService, err := newOperationService(c)
	if err != nil {
		return nil, err
	}

	blobService, err := newBlobService(c)
	if err != nil {
		return nil, err
	}

	repositoryService, err := newRepositoryService(c)
	if err != nil {
		return nil, err
	}

	diffService, err := newDiffService(c)
	if err != nil {
		return nil, err
	}

	treeService, err := newTreeService(c)
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
