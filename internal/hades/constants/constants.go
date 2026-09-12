// Package constants defines shared constants, context keys, and policy
// types used across all service handlers and middleware.
package constants

// contextKey is an unexported type to prevent context key collisions.
type contextKey string

const (
	ContextKeyUser          contextKey = "user"
	ContextKeyAuthorization contextKey = "Authorization"
)

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
