package module

import (
	"context"

	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/models"
)

func (s *Service) Downalod(ctx context.Context, refs []*models.ModuleRef) ([]*models.DownloadResponseContent, error) {

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	modules, err := s.moduleStorage.GetModulesByRefs(ctx, refs...)
	if err != nil {
		return nil, err
	}

	for _, module := range modules {
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

	commits, err := s.commitStorage.GetCommitByOwnerModule(ctx, refs)
	if err != nil {
		return nil, err
	}

	contents := []*models.DownloadResponseContent{}

	for _, commit := range commits {
		content, err := s.blobStorage.ListBlobs(ctx, commit)
		if err != nil {
			return nil, err
		}
		contents = append(contents, content...)
	}

	return contents, nil
}
