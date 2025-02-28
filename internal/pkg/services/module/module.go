package module

import (
	"context"
	"strings"

	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/internal/pkg/services/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Service struct {
	moduleDBStorage         *moduledb.ModuleStorage
	commitDBStorage         *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorizationService    *authorization.Service
	blobStorage             *blob.BlobService
}

func New(
	moduleDBStorage *moduledb.ModuleStorage,
	commitDBStorage *commitdb.CommitStorage,
	gitalyRepositoryService *repository.RepositoryService,
	gitalyOperationService *operation.OperationService,
	authorizationService *authorization.Service,
	blobStorage *blob.BlobService,
) (*Service, error) {
	return &Service{
		moduleDBStorage:         moduleDBStorage,
		commitDBStorage:         commitDBStorage,
		gitalyRepositoryService: gitalyRepositoryService,
		gitalyOperationService:  gitalyOperationService,
		blobStorage:             blobStorage,
		authorizationService:    authorizationService,
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
	module, err := s.moduleDBStorage.Create(ctx, in)
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
	err = s.commitDBStorage.Create(ctx, commit)
	if err != nil {
		return nil, err
	}

	return in, err
}

func (s *Service) GetModules(ctx context.Context, in []*models.ModuleRef) ([]*models.Module, error) {

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	modules, err := s.moduleDBStorage.GetModulesByRefs(ctx, in...)
	if err != nil {
		return nil, err
	}

	for _, module := range modules {
		// TODO check the state of the module
		if module.Visibility != models.ModuleVisibility_MODULE_VISIBILITY_PRIVATE {
			continue
		}
		moduleFullName := module.Name
		pol := &models.Policy{
			Subject: user.Username,
			Object:  string(models.REPOSITORY),
			Action:  string(models.READ),
			Domain:  moduleFullName,
		}
		can, err := s.authorizationService.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}
	}

	return modules, nil
}
