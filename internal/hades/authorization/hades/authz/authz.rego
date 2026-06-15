package hades.authz

import rego.v1

# default deny
default allow := false

# ---------------------------------------------------------------------------
# Superadmin bypass - any subject listed in data.superadmins is allowed
# everything.
# ---------------------------------------------------------------------------
allow if input.subject in data.superadmins

# ---------------------------------------------------------------------------
# Public visibility bypass - read and list are always allowed on public
# resources without a role binding.
# ---------------------------------------------------------------------------
allow if {
	input.action in {"read", "list"}
	input.visibility == "public"
}

# ---------------------------------------------------------------------------

# domain and verify the role grants the requested action on the resource.
# ---------------------------------------------------------------------------
allow if {
	some binding in data.role_bindings
	binding.subject == input.subject
	domain_matches(binding.domain, input.domain)
	role_permissions[binding.role][input.resource_type][input.action]
}

# ---------------------------------------------------------------------------
# Domain matching helpers
#   exact:    "alice/foo"  matches "alice/foo"
#   wildcard: "alice/*"   matches "alice/foo", "alice/bar", etc.
#   global:   "*"         matches any domain (reserved for superadmin bindings)
# ---------------------------------------------------------------------------
domain_matches(pattern, domain) if pattern == domain

domain_matches(pattern, domain) if {
	endswith(pattern, "/*")
	prefix := trim_suffix(pattern, "/*")
	startswith(domain, concat("", [prefix, "/"]))
}

domain_matches("*", _)

# ---------------------------------------------------------------------------
# Role–permission matrix
#
# Roles (hierarchical, each includes everything below):
#   owner       - namespace-wide (bound to "username/*")
#                 full control: create, read, list, update, push, delete,
#                 admin, transfer
#   admin       - module-level; full except ownership transfer
#   contributor - read + push commits
#   reader      - read-only (used to share private modules)
#
# Resources:
#   module  - a versioned proto module (like a git repo)
#   label   - a named pointer to a commit (branch / tag)
#   commit  - an immutable snapshot of module files
# ---------------------------------------------------------------------------

role_permissions := {
	"owner": {
		"module": {
			"create",
			"read",
			"list",
			"update",
			"push",
			"delete",
			"admin",
			"transfer",
		},
		"label": {"create", "read", "list", "update", "delete"},
		"commit": {"read", "list"},
	},
	"admin": {
		"module": {
			"create",
			"read",
			"list",
			"update",
			"push",
			"delete",
			"admin",
		},
		"label": {"create", "read", "list", "update", "delete"},
		"commit": {"read", "list"},
	},
	"contributor": {
		"module": {"read", "list", "push"},
		"label": {"read", "list"},
		"commit": {"read", "list"},
	},
	"reader": {
		"module": {"read", "list"},
		"label": {"read", "list"},
		"commit": {"read", "list"},
	},
}
