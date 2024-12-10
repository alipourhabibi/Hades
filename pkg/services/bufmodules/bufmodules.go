package bufmodules

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	"github.com/alipourhabibi/Hades/pkg/services/authorization"
	dbcommit "github.com/alipourhabibi/Hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/storage/db/module"
)

type Service struct {
	moduleStorage        *moduledb.ModuleStorage
	commitStorage        *dbcommit.CommitStorage
	authorizationService authorization.Service
}

func New(r *moduledb.ModuleStorage, c *dbcommit.CommitStorage, authorizationService *authorization.Service) (*Service, error) {
	return &Service{
		moduleStorage:        r,
		authorizationService: *authorizationService,
		commitStorage:        c,
	}, nil
}

func (s *Service) GetModules(ctx context.Context, in []*models.ModuleRef) ([]*models.Module, error) {

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	for _, ref := range in {
		moduleFullName := ref.Owner + "/" + ref.Module
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

	modules, err := s.moduleStorage.GetModulesByRefs(ctx, in...)
	if err != nil {
		return nil, err
	}

	return modules, nil
}
