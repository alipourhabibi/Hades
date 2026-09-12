package hades.authz

import rego.v1

# ---------------------------------------------------------------------------
# policy_allow is the canonical authorization check for a single policy object.
# Fields: subject, domain, resource_type, action, visibility.
#
# Both the single-eval path (allow) and the batch path (denied_indices) use
# this helper. Add new rules here only; both paths pick them up automatically.
# ---------------------------------------------------------------------------

# Superadmin bypass.
#
# data.superadmins is seeded by newHybridStore from opa.superAdmins in the
# server configuration. It used to be seeded by nothing at all, so this clause
# could never fire and granting the superadmin role granted nothing while
# reading as though it granted everything.
policy_allow(policy) if policy.subject in data.superadmins

# Public visibility bypass: read and list are always allowed on public resources.
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
#   org     - an organisation: its settings and its membership
#
# What the server actually asks about today
#
#   module:create   ModuleService.CreateModuleByName
#   module:update   ModuleService.UpdateModule
#   module:publish  ModuleService.UpdateModule, only for the private-to-public
#                   transition, which is a different decision from editing a
#                   description and is therefore a different action
#   module:push     UploadService.Upload (batched, one policy per module)
#   module:read     CheckReadAccess, on every private module any read touches
#   org:update      OrgService.UpdateOrg
#   org:admin       OrgService.AddOrgMember, RemoveOrgMember
#
# The label and commit resource rows were removed: no handler ever asked about
# them, there is no labels storage package, and the labels table is dropped by
# migration 033. Vocabulary that reads as enforcement and enforces nothing is
# worse than no vocabulary.
#
# The module actions list, delete, admin and transfer are still declared and
# still unreached. "list" in particular is never asked, so ListModules filters
# with module:read per row and reader and contributor see the same set.
# ---------------------------------------------------------------------------

role_permissions := {
	"owner": {
		"module": {
			"create",
			"read",
			"list",
			"update",
			"publish",
			"push",
			"delete",
			"admin",
			"transfer",
		},
		"org": {"read", "update", "admin"},
	},
	"admin": {
		"module": {
			"create",
			"read",
			"list",
			"update",
			"publish",
			"push",
			"delete",
			"admin",
		},
		"org": {"read", "update", "admin"},
	},
	# contributor may push, but may not publish a private module to the world.
	"contributor": {
		"module": {"read", "list", "push"},
		"org": {"read"},
	},
	"reader": {
		"module": {"read", "list"},
		"org": {"read"},
	},
}
