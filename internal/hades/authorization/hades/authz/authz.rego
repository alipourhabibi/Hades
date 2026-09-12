package hades.authz

import rego.v1

# ---------------------------------------------------------------------------
# policy_allow is the canonical authorization check for a single policy object.
# Fields: subject, domain, resource_type, action, visibility.
#
# Both the single-eval path (allow) and the batch path (denied_indices) use
# this helper. Add new rules here only. Both paths then pick them up.
# ---------------------------------------------------------------------------

# Superadmin bypass.
policy_allow(policy) if policy.subject in data.superadmins

# Public resources skip the role check. Read and list are always allowed.
policy_allow(policy) if {
	policy.action in {"read", "list"}
	policy.visibility == "public"
}

# Role-binding check: hybridStore serves data.role_bindings[subject] from
# cache (Redis or in-memory) on a per-subject basis, falling back to DB on
# cache miss. Each entry has {role, domain}.
policy_allow(policy) if {
	some binding in data.role_bindings[policy.subject]
	domain_matches(binding.domain, policy.domain)
	role_permissions[binding.role][policy.resource_type][policy.action]
}

# ---------------------------------------------------------------------------
# Single-eval path: input fields match the policy object shape exactly, so
# input can be passed directly as the policy argument.
# ---------------------------------------------------------------------------
default allow := false

allow if policy_allow(input)

# ---------------------------------------------------------------------------
# Batch-eval path: denied_indices is the set of indices in input.policies
# that are denied. Used by BatchAllow to evaluate all policies in one Eval().
# ---------------------------------------------------------------------------
denied_indices contains i if {
	some i
	policy := input.policies[i]
	not policy_allow(policy)
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
# Role and permission matrix
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
#
# What the server actually asks about today
#
# Every call site passes resource_type "module" and one of four actions:
#
#   module:create  ModuleService.CreateModuleByName
#   module:update  ModuleService.UpdateModule
#   module:push    UploadService.Upload (batched, one policy per module)
#   module:read    CheckReadAccess, on every private module any read touches
#
# The label and commit rows, and the module actions list, delete, admin and
# transfer, are not reached by any handler. They are the intended model for
# operations that do not exist yet, not permissions being enforced. Two
# consequences worth knowing:
#
#   - "list" is never asked, so ListModules filters with module:read per row.
#     reader and contributor therefore see the same set.
#   - a role's label and commit entries have no effect at all.
#
# API token scopes are narrower still: constants.scopedResources admits only
# "module", because a scope naming a resource type nobody asks about would be a
# credential that can do nothing. Widen that list in the same change that starts
# asking about the resource type.
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
