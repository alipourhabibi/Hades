package authentication

import (
	"context"
	"errors"

	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/internal/pkg/services/authorization"
	"github.com/alipourhabibi/Hades/internal/storage/db/session"
	sessiondb "github.com/alipourhabibi/Hades/internal/storage/db/session"
	userdb "github.com/alipourhabibi/Hades/internal/storage/db/user"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/utils/bcrypt"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service is the authentication service which holds the logic for authentication
type Service struct {
	userStorage          *userdb.UserStorage
	sessionStorage       *sessiondb.SessionStorage
	authorizationService *authorization.Service
}

func New(u *userdb.UserStorage, s *session.SessionStorage, authorizationService *authorization.Service) (*Service, error) {
	return &Service{
		userStorage:          u,
		sessionStorage:       s,
		authorizationService: authorizationService,
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
		return nil, pkgerr.FromGorm(err)
	}

	if exists {
		return nil, pkgerr.New("Username Exists", pkgerr.AlreadyExists)
	}

	hashedPassword, err := bcrypt.HashPassword(in.Password)
	if err != nil {
		return nil, pkgerr.FromBcrypt(err)
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
		return nil, pkgerr.FromGorm(err)
	}

	err = s.AddBasicRoles(ctx, user)
	if err != nil {
		pkgerr.FromCasbin(err)
		return nil, err
	}

	return &models.SigninResponse{
		User: user,
	}, nil
}

func (s *Service) AddBasicRoles(ctx context.Context, user *models.User) error {
	roles := []*models.Role{
		{
			User:   user.Username,
			Role:   string(models.OWNER),
			Domain: user.Username + "/*",
		},
	}
	policies := []*models.Policy{
		{
			Subject: string(models.OWNER),
			Domain:  user.Username + "/*",
			Object:  string(models.REPOSITORY),
			Action:  string(models.CREATE),
		},
	}
	err := s.authorizationService.AddPoliciesRolse(ctx, policies, roles)
	if err != nil {
		return err
	}
	return nil
}
