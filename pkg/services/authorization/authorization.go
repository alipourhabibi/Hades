package authorization

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	sessiondb "github.com/alipourhabibi/Hades/storage/db/session"
	userdb "github.com/alipourhabibi/Hades/storage/db/user"
	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
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
	allowed, err := s.casbin.Enforce(in.Subject, in.Domain, in.Object, in.Action)
	if err != nil {
		return nil, err
	}
	return &models.CanResponse{
		Allowed: allowed,
	}, nil
}

func (s *Service) AddPolicy(ctx context.Context, in *models.Policy) (*models.Policy, error) {
	_, err := s.casbin.AddPolicy(in.Subject, in.Domain, in.Object, in.Action)
	if err != nil {
		return nil, err
	}
	return in, nil
}

func (s *Service) AddPolicies(ctx context.Context, policies []*models.Policy) ([]*models.Policy, error) {
	err := s.casbin.GetAdapter().(*gormadapter.Adapter).Transaction(s.casbin, func(e casbin.IEnforcer) error {
		for _, policy := range policies {
			_, err := s.casbin.AddPolicy(policy.Subject, policy.Domain, policy.Object, policy.Action)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return policies, nil
}

func (s *Service) AddRoles(ctx context.Context, roles []*models.Role) ([]*models.Role, error) {
	err := s.casbin.GetAdapter().(*gormadapter.Adapter).Transaction(s.casbin, func(e casbin.IEnforcer) error {
		for _, role := range roles {
			_, err := s.casbin.AddRoleForUserInDomain(role.User, role.Role, role.Domain)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return roles, nil
}

func (s *Service) AddPoliciesRolse(ctx context.Context, policies []*models.Policy, roles []*models.Role) error {
	return s.casbin.GetAdapter().(*gormadapter.Adapter).Transaction(s.casbin, func(e casbin.IEnforcer) error {
		for _, policy := range policies {
			_, err := s.casbin.AddPolicy(policy.Subject, policy.Domain, policy.Object, policy.Action)
			if err != nil {
				return err
			}
		}
		for _, role := range roles {
			_, err := s.casbin.AddRoleForUserInDomain(role.User, role.Role, role.Domain)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
