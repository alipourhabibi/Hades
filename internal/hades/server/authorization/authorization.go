package authorization

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/alipourhabibi/Hades/api/gen/api/authorization/v1"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	sessiondb "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/casbin/casbin/v2"
)

type Server struct {
	authorizationv1connect.AuthorizationHandler

	logger *log.LoggerWrapper

	userStorage    *userdb.UserStorage
	sessionStorage *sessiondb.SessionStorage
	casbin         *casbin.Enforcer
}

func NewServer(
	l *log.LoggerWrapper,
	userStorage *userdb.UserStorage,
	sessionStorage *sessiondb.SessionStorage,
	casbin *casbin.Enforcer,
) *Server {
	return &Server{
		logger:         l,
		userStorage:    userStorage,
		sessionStorage: sessionStorage,
		casbin:         casbin,
	}
}

func (s *Server) UserBySession(ctx context.Context, in *connect.Request[v1.UserBySessionRequest]) (*connect.Response[v1.UserBySessionResponse], error) {
	user, ok := ctx.Value("user").(*registryv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	return &connect.Response[v1.UserBySessionResponse]{
		Msg: &v1.UserBySessionResponse{
			User: user,
		},
	}, nil
}

func (s *Server) UserFromSessionID(ctx context.Context, session string) (*registryv1.User, error) {

	user, err := s.userStorage.GetBySessionId(ctx, session)
	if err != nil {
		return nil, err
	}

	return user, nil

}

// TODO add a transaction mechanism
func (s *Server) AddBasicRoles(ctx context.Context, userName string) error {
	roles := []*constants.Role{
		{
			User:   userName,
			Role:   string(constants.OWNER),
			Domain: userName + "/*",
		},
	}
	policies := []*constants.Policy{
		{
			Subject: string(constants.OWNER),
			Domain:  userName + "/*",
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.CREATE),
		},
	}

	policyList := [][]string{}
	for _, policy := range policies {
		policyList = append(policyList, []string{policy.Subject, policy.Domain, policy.Object, policy.Action})
	}
	_, err := s.casbin.AddPolicies(policyList)
	if err != nil {
		return err
	}
	for _, role := range roles {
		_, err := s.casbin.AddRoleForUserInDomain(role.User, role.Role, role.Domain)
		if err != nil {
			return err
		}
	}
	return nil

}

func (s *Server) Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error) {
	allowed, err := s.casbin.Enforce(in.Subject, in.Domain, in.Object, in.Action)
	if err != nil {
		return nil, err
	}
	return &constants.CanResponse{
		Allowed: allowed,
	}, nil
}

// TODO add a transaction mechanism
func (s *Server) AddPoliciesRolse(ctx context.Context, policies []*constants.Policy, roles []*constants.Role) error {

	policyList := [][]string{}
	for _, policy := range policies {
		policyList = append(policyList, []string{policy.Subject, policy.Domain, policy.Object, policy.Action})
	}
	_, err := s.casbin.AddPolicies(policyList)
	if err != nil {
		return err
	}
	for _, role := range roles {
		_, err := s.casbin.AddRoleForUserInDomain(role.User, role.Role, role.Domain)
		if err != nil {
			return err
		}
	}
	return nil
}
