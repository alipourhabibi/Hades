package authorization

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	sessiondb "github.com/alipourhabibi/Hades/storage/db/session"
	userdb "github.com/alipourhabibi/Hades/storage/db/user"
	"github.com/casbin/casbin/v2"
)

type Service struct {
	userStorage    *userdb.UserStorage
	sessionStorage *sessiondb.SessionStorage
	casbin         *casbin.Enforcer
}

type ServiceConfiguration func(*Service) error

func WithCasbinEnforcer(c *casbin.Enforcer) ServiceConfiguration {
	return func(s *Service) error {
		s.casbin = c
		return nil
	}
}

func WithUserStorage(user *userdb.UserStorage) ServiceConfiguration {
	return func(s *Service) error {
		s.userStorage = user
		return nil
	}
}

func WithSessionStorage(session *sessiondb.SessionStorage) ServiceConfiguration {
	return func(s *Service) error {
		s.sessionStorage = session
		return nil
	}
}

func New(cfgs ...ServiceConfiguration) (*Service, error) {
	s := &Service{}

	for _, cfg := range cfgs {
		err := cfg(s)
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Service) Can(ctx context.Context, in *models.Policy) (*models.CanResponse, error) {
	allowed, err := s.casbin.Enforce(in.Subject, in.Object, in.Action)
	if err != nil {
		return nil, err
	}
	return &models.CanResponse{
		Allowed: allowed,
	}, nil
}

func (s *Service) AddPolicy(ctx context.Context, in *models.Policy) (*models.Policy, error) {
	_, err := s.casbin.AddPolicy(in.Subject, in.Object, in.Action)
	if err != nil {
		return nil, err
	}
	return in, nil
}
