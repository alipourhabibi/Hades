package module

import (
	"context"
	"strings"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	"github.com/alipourhabibi/Hades/pkg/services/authorization"
	moduledb "github.com/alipourhabibi/Hades/storage/db/module"
)

type Service struct {
	moduleStorage        *moduledb.ModuleStorage
	authorizationService authorization.Service
}

func New(r *moduledb.ModuleStorage, authorizationService *authorization.Service) (*Service, error) {
	return &Service{
		moduleStorage:        r,
		authorizationService: *authorizationService,
	}, nil
}

func (s *Service) CreateByNameModule(ctx context.Context, in *models.Module) (*models.Module, error) {

	in.Name = strings.ToLower(in.Name)

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	moduleFullName := user.Username + "/" + in.Name

	// TODO make it a middleware
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
	err = s.moduleStorage.Create(ctx, in)
	if err != nil {
		return nil, pkgerr.FromGorm(err)
	}

	// TODO
	// It can be ommited because we have username/* owner role
	_, err = s.authorizationService.AddRoles(ctx, []*models.Role{
		{
			User:   user.Username,
			Role:   string(models.OWNER),
			Domain: moduleFullName,
		},
	})

	return in, err
}
