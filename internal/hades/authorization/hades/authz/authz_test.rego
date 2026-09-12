package hades.authz_test

import rego.v1

# Shared bindings used across role-based tests.
#
# The shape matters and was wrong: data.role_bindings is a map keyed by
# subject, because hybridStore serves data.role_bindings[subject] per subject
# from the cache. The fixture was a flat array with a "subject" field in each
# entry, which the policy's `data.role_bindings[policy.subject]` never matched,
# so every positive assertion here failed. Nothing noticed, because no workflow
# ran these tests: see REVIEW.md R6.6.
role_bindings := {
	"alice": [{"role": "owner", "domain": "alice/*"}],
	"bob": [{"role": "admin", "domain": "alice/foo"}],
	"carol": [{"role": "contributor", "domain": "alice/foo"}],
	"dave": [{"role": "reader", "domain": "alice/foo"}],
}

# ---------------------------------------------------------------------------
# Superadmin bypass
# ---------------------------------------------------------------------------

test_superadmin_allowed if {
	data.hades.authz.allow with input as {
		"subject": "superadmin",
		"domain": "any/module",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as ["superadmin"]
		with data.role_bindings as {}
}

test_non_superadmin_not_bypassed if {
	not data.hades.authz.allow with input as {
		"subject": "mallory",
		"domain": "any/module",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as ["superadmin"]
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Public visibility bypass
# ---------------------------------------------------------------------------

test_public_read_allowed_no_binding if {
	data.hades.authz.allow with input as {
		"subject": "",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "read",
		"visibility": "public",
	}
		with data.superadmins as []
		with data.role_bindings as {}
}

test_public_list_allowed_no_binding if {
	data.hades.authz.allow with input as {
		"subject": "",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "list",
		"visibility": "public",
	}
		with data.superadmins as []
		with data.role_bindings as {}
}

test_public_delete_denied_no_binding if {
	not data.hades.authz.allow with input as {
		"subject": "anon",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "delete",
		"visibility": "public",
	}
		with data.superadmins as []
		with data.role_bindings as {}
}

# ---------------------------------------------------------------------------
# Owner role
# ---------------------------------------------------------------------------

test_owner_can_create if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/newmodule",
		"resource_type": "module",
		"action": "create",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_owner_can_transfer if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "transfer",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_owner_can_administer_their_org if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/foo",
		"resource_type": "org",
		"action": "admin",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_owner_can_publish if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "publish",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Admin role
# ---------------------------------------------------------------------------

test_admin_cannot_transfer if {
	not data.hades.authz.allow with input as {
		"subject": "bob",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "transfer",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_admin_can_delete if {
	data.hades.authz.allow with input as {
		"subject": "bob",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_admin_can_push if {
	data.hades.authz.allow with input as {
		"subject": "bob",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "push",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Contributor role
# ---------------------------------------------------------------------------

test_contributor_can_push if {
	data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "push",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_contributor_cannot_delete if {
	not data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_contributor_can_read_the_org if {
	data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "org",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# A contributor may push, but publishing a private module to the world is a
# different decision and a different action.
test_contributor_cannot_publish if {
	not data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "publish",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_contributor_cannot_administer_the_org if {
	not data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "org",
		"action": "admin",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Reader role
# ---------------------------------------------------------------------------

test_reader_can_read if {
	data.hades.authz.allow with input as {
		"subject": "dave",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_reader_cannot_push if {
	not data.hades.authz.allow with input as {
		"subject": "dave",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "push",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_reader_cannot_update_the_org if {
	not data.hades.authz.allow with input as {
		"subject": "dave",
		"domain": "alice/foo",
		"resource_type": "org",
		"action": "update",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Domain matching
# ---------------------------------------------------------------------------

test_wildcard_matches_any_submodule if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/anything",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_wildcard_does_not_match_other_namespace if {
	not data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "bob/anything",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_exact_domain_matches if {
	data.hades.authz.allow with input as {
		"subject": "bob",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_exact_domain_does_not_match_other if {
	not data.hades.authz.allow with input as {
		"subject": "bob",
		"domain": "alice/bar",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Unknown resource type
# ---------------------------------------------------------------------------

test_unknown_resource_denied if {
	not data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/foo",
		"resource_type": "unknown_resource",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# No binding = deny
# ---------------------------------------------------------------------------

test_no_binding_denied if {
	not data.hades.authz.allow with input as {
		"subject": "mallory",
		"domain": "alice/foo",
		"resource_type": "module",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

# ---------------------------------------------------------------------------
# Superadmin bypass
#
# The clause reads data.superadmins, which is seeded from opa.superAdmins in
# configuration. These tests exist because it was seeded by nothing at all, so
# it could never fire: granting the role looked like granting access and
# granted none.
# ---------------------------------------------------------------------------

test_superadmin_bypasses_every_check if {
	data.hades.authz.allow with input as {
		"subject": "root",
		"domain": "someone-else/private",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as ["root"]
		with data.role_bindings as {}
}

test_a_non_superadmin_is_still_refused if {
	not data.hades.authz.allow with input as {
		"subject": "mallory",
		"domain": "someone-else/private",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as ["root"]
		with data.role_bindings as {}
}

test_an_empty_superadmin_list_grants_nothing if {
	not data.hades.authz.allow with input as {
		"subject": "root",
		"domain": "someone-else/private",
		"resource_type": "module",
		"action": "delete",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as {}
}
