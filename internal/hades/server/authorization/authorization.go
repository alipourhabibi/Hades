// Package authorization implements the Authorization ConnectRPC service and
// provides the auth middleware used by all other handlers. It wraps the OPA
// engine to enforce role-based access control and exposes helpers for
// checking read access to modules and managing role bindings.
package authorization

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	v1 "github.com/alipourhabibi/Hades/api/gen/api/authorization/v1"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Server implements the Authorization Connect-RPC service and provides internal
// helpers used by other handlers and the auth middleware.
type Server struct {
	authorizationv1connect.UnimplementedAuthorizationHandler

	logger *log.LoggerWrapper

	engine          *authorization.Engine
	userStorage     user.Storage
	sessionStorage  session.Storage
	apiTokenStorage apitoken.Storage
	totpSecretDB    totpsecret.Storage
	cache           cache.Cache
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

// WithCache injects the cache used to bound bearer-credential presentation.
func (s *Server) WithCache(c cache.Cache) *Server {
	s.cache = c
	return s
}

// Validate reports whether the server has everything it needs to enforce
// authentication, and is called at startup so a missing dependency is a
// refusal to start rather than a security control that silently does nothing.
func (s *Server) Validate() error {
	var missing []string
	if s.userStorage == nil {
		missing = append(missing, "user storage")
	}
	if s.sessionStorage == nil {
		missing = append(missing, "session storage")
	}
	if s.apiTokenStorage == nil {
		missing = append(missing, "API token storage")
	}
	if s.totpSecretDB == nil {
		// Without it the second-factor branch cannot run. Previously the branch
		// was skipped entirely when this was nil, so an accidental rewiring
		// disabled 2FA for every session and nothing said so.
		missing = append(missing, "TOTP secret storage")
	}
	if s.engine == nil {
		missing = append(missing, "authorization engine")
	}
	if len(missing) > 0 {
		return fmt.Errorf("authorization server is missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (s *Server) UserBySession(ctx context.Context, in *connect.Request[v1.UserBySessionRequest]) (*connect.Response[v1.UserBySessionResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		return nil, connerr.Unauthenticated("not authenticated")
	}

	return &connect.Response[v1.UserBySessionResponse]{
		Msg: &v1.UserBySessionResponse{
			User: user,
		},
	}, nil
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

// Scopes describes what a credential is permitted to do.
//
// Unrestricted and an empty Values list are deliberately different things.
// Previously both were spelled "empty []string", and scopeCovers returned true
// for it, so any code path that lost the scope slice silently upgraded a
// restricted token to full authority: losing information granted access. The
// session path relied on exactly that, passing nil and meaning "unrestricted"
// by accident rather than by statement.
//
// The zero value is the safe one: no scopes, not unrestricted, deny everything.
type Scopes struct {
	// Unrestricted marks a credential that carries no scope restriction at
	// all: an interactive session, or a personal access token created before
	// scopes were enforced. It must be set deliberately.
	Unrestricted bool

	// Values are the scope entries, in the grammar scopeCovers documents.
	Values []string
}

// UnrestrictedScopes is the value to use where a credential genuinely carries
// full authority. Naming it makes each such site a statement rather than an
// omission.
func UnrestrictedScopes() Scopes { return Scopes{Unrestricted: true} }

// ScopesFromValues builds Scopes from a token's stored scope list. An empty
// list is treated as unrestricted, which is what a pre-scopes token means; a
// caller that wants deny-all should construct Scopes directly.
func ScopesFromValues(values []string) Scopes {
	if len(values) == 0 {
		return Scopes{Unrestricted: true}
	}
	return Scopes{Values: values}
}

// Allow reports whether the credential may perform resource_type:action on
// domain.
func (s Scopes) Allow(resourceType, action, domain string) bool {
	if s.Unrestricted {
		return true
	}
	return scopeCovers(s.Values, resourceType, action, domain)
}

// Restricted reports whether any scope filtering applies.
func (s Scopes) Restricted() bool { return !s.Unrestricted }

// scopeCovers reports whether scopes grants resource_type:action on domain.
//
// An empty slice covers nothing. "This credential has no restrictions" is
// Scopes.Unrestricted, not an empty list.
//
// A scope entry is "resource:action" (any domain) or "resource:action:domain"
// (one module, or one namespace via "owner/*"). The action may be "*".
//
// domain is the module full name the request targets. Pass "" when the caller
// cannot name one: a domain-restricted scope will then not match, because
// stretching it to cover an unidentified resource is exactly the mistake this
// grammar exists to prevent.
func scopeCovers(scopes []string, resourceType, action, domain string) bool {
	for _, s := range scopes {
		scopeResource, scopeAction, scopeDomain, ok := constants.ParseScope(s)
		if !ok || scopeResource != resourceType {
			continue
		}
		if scopeAction != action && scopeAction != "*" {
			continue
		}
		if constants.DomainMatches(scopeDomain, domain) {
			return true
		}
	}
	return false
}

// Can checks a single authorization policy via the OPA engine.
// If the request was made with a scoped API token the action must also be
// covered by the token's declared scopes (empty scopes = full access).
func (s *Server) Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error) {
	if !scopesFromContext(ctx).Allow(in.ResourceType, in.Action, in.Domain) {
		return &constants.CanResponse{Allowed: false, Policy: in}, nil
	}
	// Visibility is set to private for every write check. Reads are gated by
	// CheckReadAccess, which is the only path where a module being public
	// changes the answer; for create/update/push/delete, public visibility never
	// grants access, so evaluating them as private is the correct and
	// conservative input.
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
	scopes := scopesFromContext(ctx)
	for _, p := range policies {
		if !scopes.Allow(p.ResourceType, p.Action, p.Domain) {
			return &constants.CanResponse{Allowed: false, Policy: p}, nil
		}
	}
	// See Can: write actions are always evaluated as private, which is the
	// conservative input. Only CheckReadAccess branches on real visibility.
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
	// A scoped API token must carry module:read for each module it touches.
	// Without this the scope list would be enforced only on Can/BatchCan
	// (writes), so a token issued for pushing alone could read every private
	// module its owner can see. The check is per module because a scope may name
	// a single module or namespace.
	scopes := scopesFromContext(ctx)

	// Two passes. The first answers everything that needs no policy evaluation:
	// scope violations, public modules, and private modules with no caller. The
	// second asks OPA once for whatever is left.
	//
	// It used to call engine.Allow in the loop, one evaluation per module, even
	// though BatchAllow existed and BatchCan already used it. ListModules calls
	// this once per row, so a page of fifty modules was fifty evaluations.
	var pending []constants.Policy
	var pendingModules []*registryv1.Module

	for _, m := range modules {
		if !scopes.Allow(string(constants.ResourceModule), string(constants.ActionRead), m.Name) {
			// NotFound for private modules keeps their existence hidden, matching
			// the rest of this function; a public module the token may not read is
			// simply refused.
			if m.Visibility == registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE {
				return connerr.NotFound("not found")
			}
			return connerr.PermissionDenied("token is not authorised to read this module")
		}
		if m.Visibility != registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE {
			continue // public: always accessible
		}
		// Private module - must be authenticated and authorised.
		// Return NOT_FOUND regardless of whether the user is anonymous or
		// authenticated-but-unauthorised, to avoid revealing that the module exists.
		if user == nil {
			return connerr.NotFound("not found")
		}
		pending = append(pending, constants.Policy{
			Subject:      user.Username,
			Domain:       m.Name,
			ResourceType: string(constants.ResourceModule),
			Action:       string(constants.ActionRead),
			Visibility:   constants.VisibilityPrivate,
		})
		pendingModules = append(pendingModules, m)
	}

	if len(pending) == 0 {
		return nil
	}

	allowed, err := s.engine.BatchAllow(ctx, pending)
	if err != nil {
		return err
	}
	for i, ok := range allowed {
		if !ok {
			s.logger.Debug("read denied", "module", pendingModules[i].Name, "subject", pending[i].Subject)
			return connerr.NotFound("not found")
		}
	}
	return nil
}

// AddRoleBindings inserts role bindings via the OPA engine.
//
// This replaces AddPoliciesRoles, which also accepted a []*constants.Policy
// argument that was never read: any caller passing policies got a silent no-op.
func (s *Server) AddRoleBindings(ctx context.Context, roles []*constants.Role) error {
	for _, r := range roles {
		if err := s.engine.AddBinding(ctx, r.User, r.Role, r.Domain); err != nil {
			return err
		}
	}
	return nil
}
