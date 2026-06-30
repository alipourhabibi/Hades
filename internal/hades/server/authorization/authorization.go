// Package authorization implements the Authorization ConnectRPC service and
// provides the auth middleware used by all other handlers. It wraps the OPA
// engine to enforce role-based access control and exposes helpers for
// checking read access to modules and managing role bindings.
package authorization

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/alipourhabibi/Hades/api/gen/api/authorization/v1"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Server implements the Authorization Connect-RPC service and provides internal
// helpers used by other handlers and the auth middleware.
type Server struct {
	authorizationv1connect.AuthorizationHandler

	logger *log.LoggerWrapper

	engine          *authorization.Engine
	userStorage     user.Storage
	sessionStorage  session.Storage
	apiTokenStorage apitoken.Storage
	totpSecretDB    totpsecret.Storage
}

func NewServer(
	l *log.LoggerWrapper,
	userStorage user.Storage,
	sessionStorage session.Storage,
	engine *authorization.Engine,
) *Server {
	return &Server{
		logger:         l,
		userStorage:    userStorage,
		sessionStorage: sessionStorage,
		engine:         engine,
	}
}

// WithAPITokenStorage injects the API token storage for token-based auth in middleware.
func (s *Server) WithAPITokenStorage(at apitoken.Storage) *Server {
	s.apiTokenStorage = at
	return s
}

// WithTOTPSecretStorage injects the TOTP secret storage for TOTP verification in middleware.
func (s *Server) WithTOTPSecretStorage(ts totpsecret.Storage) *Server {
	s.totpSecretDB = ts
	return s
}

func (s *Server) UserBySession(ctx context.Context, in *connect.Request[v1.UserBySessionRequest]) (*connect.Response[v1.UserBySessionResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		return nil, connErr.Internal("missing user in context")
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

// AddBasicRoles inserts the namespace-wide owner binding for a new user and
// reloads the OPA store. This is the non-transactional variant.
func (s *Server) AddBasicRoles(ctx context.Context, userName string) error {
	return s.engine.AddBinding(ctx, userName, constants.RoleOwner, userName+"/*")
}

// AddBasicRolesInTx inserts the namespace-wide owner binding using the
// transaction injected into ctx. The caller must call ReloadPolicy after commit.
func (s *Server) AddBasicRolesInTx(ctx context.Context, userName string) error {
	return s.engine.AddBindingInTx(ctx, userName, constants.RoleOwner, userName+"/*")
}

// ReloadPolicy syncs the in-memory OPA store from the database.
func (s *Server) ReloadPolicy() error {
	return s.engine.Reload(context.Background())
}

// normalizeResource maps legacy object names to OPA resource types.
// Handlers historically used "repository" (constants.REPOSITORY); the Rego
// policy uses "module" (constants.ResourceModule).
func normalizeResource(obj string) string {
	if obj == string(constants.REPOSITORY) {
		return string(constants.ResourceModule)
	}
	return obj
}

// Can checks a single authorization policy via the OPA engine.
func (s *Server) Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error) {
	input := authorization.Input{
		Subject:      in.Subject,
		Domain:       in.Domain,
		ResourceType: normalizeResource(in.Object),
		Action:       in.Action,
		Visibility:   constants.VisibilityPrivate,
	}
	allowed, err := s.engine.Allow(ctx, input)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return &constants.CanResponse{Allowed: false, Policy: in}, nil
	}
	return &constants.CanResponse{Allowed: true}, nil
}

// BatchCan checks multiple policies, returning the first denied one.
func (s *Server) BatchCan(ctx context.Context, policies []*constants.Policy) (*constants.CanResponse, error) {
	if len(policies) == 0 {
		return &constants.CanResponse{Allowed: true}, nil
	}
	for _, p := range policies {
		resp, err := s.Can(ctx, p)
		if err != nil {
			return nil, err
		}
		if !resp.Allowed {
			return resp, nil
		}
	}
	return &constants.CanResponse{Allowed: true}, nil
}

// CheckReadAccess returns an error for the first private module the caller is
// not authorised to read. Public modules pass without an OPA call.
//
// user may be nil (anonymous). An anonymous caller can read public modules but
// cannot access private ones - those are surfaced as NotFound so that the
// existence of private modules is never revealed to unauthenticated callers.
func (s *Server) CheckReadAccess(ctx context.Context, user *registryv1.User, modules []*registryv1.Module) error {
	for _, m := range modules {
		if m.Visibility != registryv1.EVisibility_E_VISIBILITY_PRIVATE {
			continue // public: always accessible
		}
		// Private module - must be authenticated.
		if user == nil {
			return connErr.NotFound("not found")
		}
		input := authorization.Input{
			Subject:      user.Username,
			Domain:       m.Name,
			ResourceType: string(constants.ResourceModule),
			Action:       string(constants.ActionRead),
			Visibility:   constants.VisibilityPrivate,
		}
		allowed, err := s.engine.Allow(ctx, input)
		if err != nil {
			return err
		}
		if !allowed {
			return connErr.PermissionDenied("permission denied reading module " + m.Name)
		}
	}
	return nil
}

// AddPoliciesRoles inserts arbitrary role bindings via the OPA engine.
func (s *Server) AddPoliciesRoles(ctx context.Context, policies []*constants.Policy, roles []*constants.Role) error {
	for _, r := range roles {
		if err := s.engine.AddBinding(ctx, r.User, r.Role, r.Domain); err != nil {
			return err
		}
	}
	return nil
}
