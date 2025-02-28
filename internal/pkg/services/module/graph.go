package module

import (
	"context"

	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/models"
)

func (s *Service) GetGraph(ctx context.Context, req []*models.ModuleRef) ([]*models.Commit, error) {

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	modules, err := s.moduleStorage.GetModulesByRefs(ctx, req...)
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

	// TODO this should be changed and get the graph
	return s.commitStorage.GetCommitByOwnerModule(ctx, req)
}
