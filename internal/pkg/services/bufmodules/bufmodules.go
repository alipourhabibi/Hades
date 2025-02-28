package bufmodules

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/internal/pkg/services/authorization"
	dbcommit "github.com/alipourhabibi/Hades/internal/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/storage/db/module"
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

	modules, err := s.moduleStorage.GetModulesByRefs(ctx, in...)
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
