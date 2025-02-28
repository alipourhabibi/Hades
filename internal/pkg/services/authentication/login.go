package authentication

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/utils/bcrypt"
	"github.com/google/uuid"
)

func (s *Service) Login(ctx context.Context, in *models.LoginRequest) (*models.LoginResponse, error) {
	resp, err := s.userStorage.GetByUsername(ctx, in.Username)
	if err != nil {
		return nil, pkgerr.FromGorm(err)
	}

	err = bcrypt.CheckPasswordHash(in.Password, resp.Password)
	if err != nil {
		return nil, pkgerr.FromBcrypt(err)
	}

	id := uuid.New()
	err = s.sessionStorage.Create(ctx, &models.Session{
		ID:         id,
		UserID:     resp.ID,
		AuthModule: "session",
		// TODO read it from config
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		return nil, pkgerr.FromGorm(err)
	}

	return &models.LoginResponse{
		Token: id.String(),
	}, nil
}
