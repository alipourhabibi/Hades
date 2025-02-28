package authorization

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	utilserr "github.com/alipourhabibi/Hades/utils/errors"
)

func (s *Service) UserBySession(ctx context.Context, id string) (*models.User, error) {
	user, err := s.userStorage.GetBySessionId(ctx, id)
	if err != nil {
		return nil, utilserr.MapUserAuthError(err)
	}
	return user, nil
}
