// Package constants defines shared constants, context keys, and policy
// types used across all service handlers and middleware.
package constants

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

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

// nameRe is the character set for a namespace or module name: lowercase
// alphanumerics, with dashes, underscores and dots allowed in the interior
// only. A single character is allowed by the second alternative.
//
// Anchored at both ends, so nothing containing "/", "\", whitespace or a path
// segment can match. Refusing a leading or trailing punctuation character is
// what keeps ".git", "..", "-dash" and "name." out.
var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

// MaxNameLength bounds a namespace or module name. It matches the username
// bound declared in auth.proto.
const MaxNameLength = 32

// ValidateName checks a name that will become a path segment: a username, an
// organisation name, or a module name.
//
// A module name reaches the filesystem. gogit builds a repository path with
// filepath.Join(root, "<owner>/<name>"), and filepath.Join calls Clean, which
// resolves ".." rather than rejecting it: a module named "../escape" produced a
// git repository one level above the owner's namespace, in the directory where
// owner namespaces live. "a/b" created a nested subtree, ".git" created a
// directory git itself treats specially, and the Gitaly backend passes the same
// string as a RelativePath. Module names were not checked at all before this,
// even though the first path segment, the username, was.
//
// The check belongs in the handler, before any storage or git call, so a
// rejected name never leaves a half-created record: "has space" was accepted,
// got a database row, and failed to get a repository, so the record and the
// storage disagreed from the moment it existed.
//
// The name is expected already lowercased and trimmed by the caller, which is
// what every handler does; an uppercase letter is rejected rather than folded,
// so a caller that forgot finds out.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("name is required")
	case len(name) > MaxNameLength:
		return fmt.Errorf("name must be at most %d characters", MaxNameLength)
	case !nameRe.MatchString(name):
		return errors.New("name must be lowercase letters, digits, dashes, underscores or dots, and must start and end with a letter or digit")
	case IsReservedName(name):
		return errors.New("name is reserved")
	}
	return nil
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
	// RoleSuperAdmin bypasses every policy check. The subjects it applies to
	// come from opa.superAdmins in configuration, which is what the policy's
	// data.superadmins clause reads; before that was wired the clause could
	// never fire and granting the role granted nothing.
	RoleSuperAdmin = "superadmin"
)

// ResourceType identifies the kind of resource in an OPA policy check.
type ResourceType string

const (
	ResourceModule ResourceType = "module"
	// ResourceOrg covers organisation membership and settings. Organisation
	// mutations used to authorise themselves against the org_memberships table
	// directly, which left two sources of truth for the same question.
	ResourceOrg ResourceType = "org"
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

	// ActionPublish is the private-to-public visibility transition.
	//
	// It is checked separately from ActionUpdate because publishing a private
	// schema to the world is a materially different decision from editing its
	// description, and a contributor who legitimately holds update rights
	// should not necessarily hold it.
	ActionPublish Action = "publish"
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
// A resource type belongs here once some authorization check actually asks
// about it; a scope naming a type nobody asks about matches nothing, so the
// token it belongs to is dead on arrival while looking deliberate.
var scopedResources = []ResourceType{ResourceModule, ResourceOrg}

// knownScopes is the complete set of scope strings an API token may declare.
// A wildcard form ("module:*") is accepted for every resource type.
var knownScopes = func() map[string]struct{} {
	actions := []Action{
		ActionCreate, ActionRead, ActionList, ActionUpdate,
		ActionPush, ActionDelete, ActionAdmin, ActionTransfer, ActionPublish,
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
