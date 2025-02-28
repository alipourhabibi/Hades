package module

import (
	"context"
	"strings"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/internal/pkg/services/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Service struct {
	moduleStorage           *moduledb.ModuleStorage
	commitStorage           *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorizationService    *authorization.Service
}

func New(r *moduledb.ModuleStorage, c *commitdb.CommitStorage, gitalyRepositoryService *repository.RepositoryService, o *operation.OperationService, authorizationService *authorization.Service) (*Service, error) {
	return &Service{
		moduleStorage:           r,
		authorizationService:    authorizationService,
		commitStorage:           c,
		gitalyRepositoryService: gitalyRepositoryService,
		gitalyOperationService:  o,
	}, nil
}

func (s *Service) CreateByNameModule(ctx context.Context, in *models.Module) (*models.Module, error) {

	in.Name = strings.ToLower(in.Name)

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	moduleFullName := user.Username + "/" + in.Name

	can, err := s.authorizationService.Can(ctx, &models.Policy{
		Subject: user.Username,
		Object:  string(models.REPOSITORY),
		Action:  string(models.CREATE),
		Domain:  moduleFullName,
	})
	if err != nil {
		return nil, err
	}

	if !can.Allowed {
		return nil, pkgerr.New("User is not allowed to create this repo", pkgerr.PermissionDenied)
	}

	in.OwnerID = user.ID
	in.Name = moduleFullName
	module, err := s.moduleStorage.Create(ctx, in)
	if err != nil {
		return nil, pkgerr.FromGorm(err)
	}

	// TODO
	// It can be ommited because we have username/* owner role
	roles := []*models.Role{
		{
			User:   user.Username,
			Role:   string(models.OWNER),
			Domain: moduleFullName,
		},
	}
	policies := []*models.Policy{
		{
			Subject: user.Username,
			Domain:  moduleFullName,
			Object:  string(models.REPOSITORY),
			Action:  string(models.PUSH),
		},
		{
			Subject: user.Username,
			Domain:  moduleFullName,
			Object:  string(models.REPOSITORY),
			Action:  string(models.READ),
		},
	}
	err = s.authorizationService.AddPoliciesRolse(ctx, policies, roles)
	if err != nil {
		return nil, err
	}

	// TODO should create a commit in gitaly as well
	// Init commit
	err = s.gitalyRepositoryService.CreateRepository(ctx, in)
	if err != nil {
		return nil, err
	}

	content := &models.UploadRequest_Content{
		// ModuleRef: &models.ModuleRef{
		// 	Owner:  module.Owner.Username,
		// 	Module: module.Name,
		// },
		Files: []*models.File{
			{
				Path:    "README.md",
				Content: []byte(""),
			},
		},
	}

	paths := []string{}
	digestValue, err := shake256.DigestFiles(content.Files)

	hash, err := s.gitalyOperationService.UserCommitFiles(ctx, module, content.Files, user, paths, digestValue.String())
	if err != nil {
		return nil, err
	}

	commit := &models.Commit{
		ID:              uuid.New(),
		CommitHash:      hash,
		OwnerID:         user.ID,
		ModuleID:        module.ID,
		DigestType:      models.DigestType_B5,
		CreatedByUserID: user.ID,
	}
	err = s.commitStorage.Create(ctx, commit)
	if err != nil {
		return nil, err
	}

	return in, err
}
