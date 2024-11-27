package authentication

import (
	"context"
	"errors"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	userdb "github.com/alipourhabibi/Hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/bcrypt"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service is the authentication service which holds the logic for authentication
type Service struct {
	userStorage *userdb.UserStorage
}

func New(u *userdb.UserStorage) (*Service, error) {
	return &Service{
		userStorage: u,
	}, nil
}

func (s *Service) isUserExists(ctx context.Context, username string) (bool, error) {
	_, err := s.userStorage.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Service) Signin(ctx context.Context, in *models.SigninRequest) (*models.SigninResponse, error) {
	// TODO maybe better ways to handle it ?
	exists, err := s.isUserExists(ctx, in.Username)
	if err != nil {
		return nil, err
	}

	if exists {
		return nil, pkgerr.UsernameExists
	}

	hashedPassword, err := bcrypt.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:          uuid.New(),
		Username:    in.Username,
		Type:        models.UserType_USER_TYPE_USER,
		State:       models.UserState_USER_STATE_ACTIVE,
		Description: in.Description,
		Password:    hashedPassword,
	}

	err = s.userStorage.Create(ctx, user)
	if err != nil {
		return nil, err
	}

	return &models.SigninResponse{
		User: user,
	}, nil
}
