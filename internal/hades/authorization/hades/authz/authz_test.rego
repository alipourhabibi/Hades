package hades.authz_test

import rego.v1

# Shared bindings used across role-based tests.
role_bindings := [
	{"subject": "alice", "role": "owner", "domain": "alice/*"},
	{"subject": "bob", "role": "admin", "domain": "alice/foo"},
	{"subject": "carol", "role": "contributor", "domain": "alice/foo"},
	{"subject": "dave", "role": "reader", "domain": "alice/foo"},
]

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
		with data.role_bindings as []
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
		with data.role_bindings as []
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
		with data.role_bindings as []
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
		with data.role_bindings as []
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

test_owner_can_manage_labels if {
	data.hades.authz.allow with input as {
		"subject": "alice",
		"domain": "alice/foo",
		"resource_type": "label",
		"action": "delete",
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

test_contributor_can_read_commits if {
	data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "commit",
		"action": "read",
		"visibility": "private",
	}
		with data.superadmins as []
		with data.role_bindings as role_bindings
}

test_contributor_cannot_write_labels if {
	not data.hades.authz.allow with input as {
		"subject": "carol",
		"domain": "alice/foo",
		"resource_type": "label",
		"action": "create",
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

test_reader_cannot_create_labels if {
	not data.hades.authz.allow with input as {
		"subject": "dave",
		"domain": "alice/foo",
		"resource_type": "label",
		"action": "create",
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
