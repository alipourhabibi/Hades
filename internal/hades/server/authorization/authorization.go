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
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
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
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		return nil, connErr.Unauthenticated("not authenticated")
	}

	return &connect.Response[v1.UserBySessionResponse]{
		Msg: &v1.UserBySessionResponse{
			User: user,
		},
	}, nil
}

func (s *Server) UserFromSessionID(ctx context.Context, session string) (*identityv1.User, error) {
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

// AddOrgOwner grants the owner role over all resources in an org namespace.
func (s *Server) AddOrgOwner(ctx context.Context, subject, orgName string) error {
	return s.engine.AddBinding(ctx, subject, constants.RoleOwner, orgName+"/*")
}

// AddOrgMemberBinding grants a role (contributor or admin) to a user over all
// resources in an org namespace.
func (s *Server) AddOrgMemberBinding(ctx context.Context, subject, role, orgName string) error {
	return s.engine.AddBinding(ctx, subject, role, orgName+"/*")
}

// DeleteOrgBinding removes all bindings for subject within the org namespace.
func (s *Server) DeleteOrgBinding(ctx context.Context, subject, orgName string) error {
	return s.engine.DeleteBinding(ctx, subject, orgName+"/*")
}

// scopeCovers reports whether scopes grants resource_type:action.
// An empty scopes slice means unrestricted (full access).
// Wildcard "resource_type:*" covers any action on that resource.
func scopeCovers(scopes []string, resourceType, action string) bool {
	if len(scopes) == 0 {
		return true
	}
	exact := resourceType + ":" + action
	wildcard := resourceType + ":*"
	for _, s := range scopes {
		if s == exact || s == wildcard {
			return true
		}
	}
	return false
}

// Can checks a single authorization policy via the OPA engine.
// If the request was made with a scoped API token the action must also be
// covered by the token's declared scopes (empty scopes = full access).
func (s *Server) Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error) {
	if scopes, ok := ctx.Value(constants.ContextKeyTokenScopes).([]string); ok && len(scopes) > 0 {
		if !scopeCovers(scopes, in.ResourceType, in.Action) {
			return &constants.CanResponse{Allowed: false, Policy: in}, nil
		}
	}
	p := *in
	p.Visibility = constants.VisibilityPrivate
	allowed, err := s.engine.Allow(ctx, p)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return &constants.CanResponse{Allowed: false, Policy: in}, nil
	}
	return &constants.CanResponse{Allowed: true}, nil
}

// BatchCan evaluates all policies in a single OPA call, returning the first
// denied policy (preserving input order). Uses engine.BatchAllow which calls
// denied_indices in one Eval() rather than N sequential Allow() calls.
// Token scope restrictions are applied before the OPA check.
func (s *Server) BatchCan(ctx context.Context, policies []*constants.Policy) (*constants.CanResponse, error) {
	if len(policies) == 0 {
		return &constants.CanResponse{Allowed: true}, nil
	}
	scopes, _ := ctx.Value(constants.ContextKeyTokenScopes).([]string)
	if len(scopes) > 0 {
		for _, p := range policies {
			if !scopeCovers(scopes, p.ResourceType, p.Action) {
				return &constants.CanResponse{Allowed: false, Policy: p}, nil
			}
		}
	}
	inputs := make([]constants.Policy, len(policies))
	for i, p := range policies {
		inputs[i] = *p
		inputs[i].Visibility = constants.VisibilityPrivate
	}
	results, err := s.engine.BatchAllow(ctx, inputs)
	if err != nil {
		return nil, err
	}
	for i, allowed := range results {
		if !allowed {
			return &constants.CanResponse{Allowed: false, Policy: policies[i]}, nil
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
func (s *Server) CheckReadAccess(ctx context.Context, user *identityv1.User, modules []*registryv1.Module) error {
	for _, m := range modules {
		if m.Visibility != registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE {
			continue // public: always accessible
		}
		// Private module - must be authenticated and authorised.
		// Return NOT_FOUND regardless of whether the user is anonymous or
		// authenticated-but-unauthorised, to avoid revealing that the module exists.
		if user == nil {
			return connErr.NotFound("not found")
		}
		allowed, err := s.engine.Allow(ctx, constants.Policy{
			Subject:      user.Username,
			Domain:       m.Name,
			ResourceType: string(constants.ResourceModule),
			Action:       string(constants.ActionRead),
			Visibility:   constants.VisibilityPrivate,
		})
		if err != nil {
			return err
		}
		if !allowed {
			return connErr.NotFound("not found")
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
