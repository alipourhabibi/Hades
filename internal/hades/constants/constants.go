// Package constants defines shared constants, context keys, and policy
// types used across all service handlers and middleware.
package constants

import "strings"

// contextKey is an unexported type to prevent context key collisions.
type contextKey string

const (
	ContextKeyUser          contextKey = "user"
	ContextKeyAuthorization contextKey = "Authorization"
	ContextKeyTokenScopes   contextKey = "token_scopes"
)

// reservedNames are namespace names that may not be claimed by a user or an
// organisation. Both live in the same `users` table and both become the first
// path segment of a module name, so a name that collides with a real route
// (for example "go", "gen", "settings") would shadow it.
var reservedNames = map[string]struct{}{
	"settings": {}, "login": {}, "signup": {}, "search": {},
	"verify-email": {}, "api": {}, "app": {},
	"go": {}, "gen": {}, "oauth2": {}, "buf": {}, "hades": {},
	"admin": {}, "administrator": {}, "root": {}, "system": {},
	"help": {}, "support": {}, "about": {}, "pricing": {},
	"terms": {}, "privacy": {}, "security": {}, "status": {},
	"user": {}, "users": {}, "org": {}, "orgs": {},
	"team": {}, "teams": {}, "me": {}, "null": {}, "undefined": {},
	"new": {}, "home": {},
}

// IsReservedName reports whether name may not be registered as a username or
// an organisation name. The comparison is case-insensitive.
func IsReservedName(name string) bool {
	_, found := reservedNames[strings.ToLower(strings.TrimSpace(name))]
	return found
}

type Action string

const (
	CREATE Action = "create"
	PUSH   Action = "push"
	READ   Action = "read"
)

// Roles recognised by the OPA authorization engine.
const (
	RoleOwner       = "owner"
	RoleAdmin       = "admin"
	RoleContributor = "contributor"
	RoleReader      = "reader"
	RoleSuperAdmin  = "superadmin"
)

// ResourceType identifies the kind of resource in an OPA policy check.
type ResourceType string

const (
	ResourceModule    ResourceType = "module"
	ResourceLabel     ResourceType = "label"
	ResourceCommit    ResourceType = "commit"
	ResourceNamespace ResourceType = "namespace"
)

// Extended action set used by the OPA policy.
const (
	ActionCreate   Action = "create"
	ActionRead     Action = "read"
	ActionList     Action = "list"
	ActionUpdate   Action = "update"
	ActionPush     Action = "push"
	ActionDelete   Action = "delete"
	ActionAdmin    Action = "admin"
	ActionTransfer Action = "transfer"
)

// Visibility levels used as OPA input.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// Scope returns the namespace-wide "resource_type:action" API token scope.
func Scope(resource ResourceType, action Action) string {
	return string(resource) + ":" + string(action)
}

// ScopeOn returns an API token scope restricted to a single domain:
// "resource_type:action:domain".
//
// domain is a module full name ("alice/mymod") or a namespace wildcard
// ("alice/*"). A scope with no domain part applies to every domain, which is
// what the two-part form means.
func ScopeOn(resource ResourceType, action Action, domain string) string {
	return Scope(resource, action) + ":" + domain
}

// ParseScope splits a scope string into its parts.
//
// domain is "" for the two-part form, meaning the scope is not restricted to
// any particular resource.
func ParseScope(scope string) (resource, action, domain string, ok bool) {
	parts := strings.SplitN(scope, ":", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	if len(parts) == 3 {
		if parts[2] == "" {
			return "", "", "", false
		}
		domain = parts[2]
	}
	return parts[0], parts[1], domain, true
}

// DomainMatches reports whether a scope's domain part covers domain.
//
// pattern is either an exact module full name ("alice/mymod") or a namespace
// wildcard ("alice/*"). An empty pattern means the scope carries no domain
// restriction and therefore covers everything.
//
// An empty domain means the caller could not name the resource being accessed.
// Only an unrestricted scope matches in that case: a domain-restricted scope
// must never be stretched to cover a resource nobody identified.
func DomainMatches(pattern, domain string) bool {
	if pattern == "" {
		return true
	}
	if domain == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if ns, found := strings.CutSuffix(pattern, "/*"); found {
		owner, _, hasSlash := strings.Cut(domain, "/")
		return hasSlash && owner == ns
	}
	return false
}

// scopedResources are the resource types a token scope may name.
//
// Only "module" is listed, even though ResourceLabel, ResourceCommit and
// ResourceNamespace exist as policy vocabulary. No authorization check ever
// asks about those three: every call site passes ResourceModule, so a scope
// naming one of them matches nothing and the token it belongs to can do
// nothing. Accepting such a scope would hand out a credential that is dead on
// arrival and looks deliberate. Extend this list in the same change that starts
// enforcing the resource type, not before.
var scopedResources = []ResourceType{ResourceModule}

// knownScopes is the complete set of scope strings an API token may declare.
// A wildcard form ("module:*") is accepted for every resource type.
var knownScopes = func() map[string]struct{} {
	actions := []Action{
		ActionCreate, ActionRead, ActionList, ActionUpdate,
		ActionPush, ActionDelete, ActionAdmin, ActionTransfer,
	}
	out := make(map[string]struct{}, len(scopedResources)*(len(actions)+1))
	for _, r := range scopedResources {
		out[string(r)+":*"] = struct{}{}
		for _, a := range actions {
			out[Scope(r, a)] = struct{}{}
		}
	}
	return out
}()

// IsKnownScope reports whether scope is a well-formed API token scope.
//
// Both forms are accepted:
//
//	resource:action           applies to every domain
//	resource:action:domain    applies to one module or one namespace
//
// The domain part is validated for shape only. It is deliberately not checked
// against existing modules: a token may legitimately be issued for a module
// that has not been created yet, and rejecting unknown names here would turn
// token creation into a module-existence oracle.
func IsKnownScope(scope string) bool {
	resource, action, domain, ok := ParseScope(scope)
	if !ok {
		return false
	}
	if _, found := knownScopes[resource+":"+action]; !found {
		return false
	}
	if domain == "" {
		return true
	}
	return isValidScopeDomain(domain)
}

// isValidScopeDomain reports whether domain is "owner/name" or "owner/*".
func isValidScopeDomain(domain string) bool {
	owner, name, found := strings.Cut(domain, "/")
	if !found || owner == "" || name == "" {
		return false
	}
	// Only the name may be a wildcard, and only as the whole segment: an owner
	// wildcard would make the scope cover the entire registry, which is what
	// the two-part form already expresses.
	if strings.ContainsAny(owner, "*/") {
		return false
	}
	return name == "*" || !strings.ContainsAny(name, "*/")
}

type CanResponse struct {
	Allowed bool
	Policy  *Policy // set to the denied policy when Allowed is false; nil otherwise
}

// Policy is the canonical authorization input type used by the OPA engine and
// all server-layer Can/BatchCan callers. JSON tags match the Rego input field
// names so the struct can be passed directly to rego.EvalInput without an
// intermediate conversion struct.
type Policy struct {
	Subject      string `json:"subject"`
	Domain       string `json:"domain"`
	ResourceType string `json:"resource_type"`
	Action       string `json:"action"`
	Visibility   string `json:"visibility"`
}

type Role struct {
	User   string
	Role   string
	Domain string
}
