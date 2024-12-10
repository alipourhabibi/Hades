package gitaly

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/storage/gitaly/commit"
	"github.com/alipourhabibi/Hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/storage/gitaly/repository"
)

type StorageService struct {
	CommitService     *commit.CommitService
	OperattionService *operation.OperationService
	RepositoryService *repository.RepositoryService
}

func NewStorage(c config.Gitaly) (*StorageService, error) {
	commitService, err := commit.NewDefault(c)
	if err != nil {
		return nil, err
	}

	operationService, err := operation.NewDefault(c)
	if err != nil {
		return nil, err
	}

	repositoryService, err := repository.NewDefault(c)
	if err != nil {
		return nil, err
	}

	return &StorageService{
		CommitService:     commitService,
		OperattionService: operationService,
		RepositoryService: repositoryService,
	}, nil
}
