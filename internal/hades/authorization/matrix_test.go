package authorization

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
)

// Exhaustive coverage of the role and permission matrix in authz.rego: every
// role against every resource type against every action, plus the same grid for
// a subject with no binding at all.
//
// The individual tests in engine_test.go check the interesting cases and say
// why they matter. This file checks all of them, so that editing the rego to
// fix one cell cannot quietly change another. The expectation table below is
// written out by hand rather than derived from the policy, because a table
// generated from the thing under test proves nothing.

// grants is the expected permission set, transcribed from authz.rego.
var grants = map[string]map[string][]string{
	"owner": {
		"module": {"create", "read", "list", "update", "publish", "push", "delete", "admin", "transfer"},
		"org":    {"read", "update", "admin"},
	},
	"admin": {
		"module": {"create", "read", "list", "update", "publish", "push", "delete", "admin"},
		"org":    {"read", "update", "admin"},
	},
	"contributor": {
		"module": {"read", "list", "push"},
		"org":    {"read"},
	},
	"reader": {
		"module": {"read", "list"},
		"org":    {"read"},
	},
}

func allRoles() []string { return []string{"owner", "admin", "contributor", "reader"} }
func allResources() []string {
	return []string{
		string(constants.ResourceModule),
		string(constants.ResourceOrg),
	}
}

// unknownResource is a resource type the policy has no rows for. It stands in
// for the vocabulary that used to exist without enforcement ("label",
// "commit", "namespace"), which is now removed.
const unknownResource = "namespace"

func allActions() []string {
	return []string{
		string(constants.ActionCreate), string(constants.ActionRead), string(constants.ActionList),
		string(constants.ActionUpdate), string(constants.ActionPush), string(constants.ActionDelete),
		string(constants.ActionAdmin), string(constants.ActionTransfer),
		string(constants.ActionPublish),
	}
}

func expected(role, resource, action string) bool {
	for _, granted := range grants[role][resource] {
		if granted == action {
			return true
		}
	}
	return false
}

// TestMatrix_EveryRoleResourceAction walks the full grid against a private
// module, where the binding is the only thing that can grant access.
func TestMatrix_EveryRoleResourceAction(t *testing.T) {
	for _, role := range allRoles() {
		e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: role, Domain: "alice/*"})
		for _, resource := range allResources() {
			for _, action := range allActions() {
				name := fmt.Sprintf("%s/%s/%s", role, resource, action)
				t.Run(name, func(t *testing.T) {
					got := allow(t, e, "alice", "alice/mymod", resource, action, "private")
					assert.Equal(t, expected(role, resource, action), got,
						"role %q on %s:%s", role, resource, action)
				})
			}
		}
	}
}

// TestMatrix_UnknownResourceIsGrantedByNoRole is the deny-by-default rule for
// a resource type the policy has no rows for. It is why the scope vocabulary
// lists only the types some check actually asks about: a scope naming a type no
// role grants would be a credential that can do nothing while looking
// deliberate.
func TestMatrix_UnknownResourceIsGrantedByNoRole(t *testing.T) {
	for _, role := range allRoles() {
		e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: role, Domain: "alice/*"})
		for _, action := range allActions() {
			assert.False(t,
				allow(t, e, "alice", "alice/mymod", unknownResource, action, "private"),
				"role %q must not grant %s:%s", role, unknownResource, action)
		}
	}
}

// TestMatrix_NoBindingGrantsNothingOnAPrivateResource is the deny-by-default
// half of the grid.
func TestMatrix_NoBindingGrantsNothingOnAPrivateResource(t *testing.T) {
	e := engine(t)
	for _, resource := range allResources() {
		for _, action := range allActions() {
			assert.False(t, allow(t, e, "stranger", "alice/mymod", resource, action, "private"),
				"a subject with no binding must not get %s:%s", resource, action)
		}
	}
}

// TestMatrix_PublicVisibilityGrantsReadAndListOnly pins the public bypass at
// exactly two actions. Widening it to update or push would hand every module in
// the registry to anyone who can reach the port.
func TestMatrix_PublicVisibilityGrantsReadAndListOnly(t *testing.T) {
	e := engine(t)
	for _, resource := range allResources() {
		for _, action := range allActions() {
			want := action == string(constants.ActionRead) || action == string(constants.ActionList)
			assert.Equal(t, want, allow(t, e, "stranger", "alice/mymod", resource, action, "public"),
				"public %s:%s", resource, action)
		}
	}
}

// TestMatrix_BindingDoesNotLeakAcrossNamespaces walks the same grid against a
// module in a namespace the subject holds no binding for.
func TestMatrix_BindingDoesNotLeakAcrossNamespaces(t *testing.T) {
	for _, role := range allRoles() {
		e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: role, Domain: "alice/*"})
		for _, resource := range allResources() {
			for _, action := range allActions() {
				assert.False(t, allow(t, e, "alice", "bob/theirs", resource, action, "private"),
					"role %q over alice/* must not reach bob/theirs for %s:%s", role, resource, action)
			}
		}
	}
}

// TestMatrix_ExactDomainBindingCoversOnlyThatModule covers the non-wildcard
// binding form, which is how a single private module is shared.
func TestMatrix_ExactDomainBindingCoversOnlyThatModule(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "reader", Domain: "alice/shared"})

	assert.True(t, allow(t, e, "carol", "alice/shared", "module", "read", "private"))
	assert.False(t, allow(t, e, "carol", "alice/other", "module", "read", "private"),
		"an exact binding must not spread to a sibling module")
	assert.False(t, allow(t, e, "carol", "alice/shared", "module", "push", "private"),
		"reader is read-only even on the module it names")
}

// TestMatrix_RolesAreOrderedByStrength states the hierarchy the rego comment
// claims, as an assertion rather than prose: each role's module permissions are
// a superset of the next one down.
func TestMatrix_RolesAreOrderedByStrength(t *testing.T) {
	ordered := []string{"owner", "admin", "contributor", "reader"}
	for i := 0; i < len(ordered)-1; i++ {
		stronger, weaker := ordered[i], ordered[i+1]
		strongEngine := engine(t, opabinding.RoleBinding{Subject: "alice", Role: stronger, Domain: "alice/*"})
		weakEngine := engine(t, opabinding.RoleBinding{Subject: "alice", Role: weaker, Domain: "alice/*"})

		for _, action := range allActions() {
			if allow(t, weakEngine, "alice", "alice/mymod", "module", action, "private") {
				assert.True(t, allow(t, strongEngine, "alice", "alice/mymod", "module", action, "private"),
					"%s grants module:%s but %s does not, breaking the hierarchy", weaker, action, stronger)
			}
		}
	}
}

// TestMatrix_OnlyOwnerCanTransfer is the single permission that separates owner
// from admin, so it is worth naming on its own.
func TestMatrix_OnlyOwnerCanTransfer(t *testing.T) {
	for _, role := range allRoles() {
		e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: role, Domain: "alice/*"})
		got := allow(t, e, "alice", "alice/mymod", "module", string(constants.ActionTransfer), "private")
		assert.Equal(t, role == "owner", got, "role %q and module:transfer", role)
	}
}

// TestMatrix_UnknownRoleGrantsNothing covers a binding row naming a role the
// policy does not define, which is what a typo or a rolled-back policy change
// looks like in the database.
func TestMatrix_UnknownRoleGrantsNothing(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "superuser", Domain: "alice/*"})
	for _, resource := range allResources() {
		for _, action := range allActions() {
			assert.False(t, allow(t, e, "alice", "alice/mymod", resource, action, "private"),
				"an unrecognised role must fail closed for %s:%s", resource, action)
		}
	}
}
