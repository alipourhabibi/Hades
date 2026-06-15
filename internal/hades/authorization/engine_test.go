package authorization

import (
	"context"
	"testing"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore is an in-memory bindingStore used in tests; no database required.
type fakeStore struct {
	bindings []opabinding.RoleBinding
}

func (f *fakeStore) ListAll(_ context.Context) ([]opabinding.RoleBinding, error) {
	cp := make([]opabinding.RoleBinding, len(f.bindings))
	copy(cp, f.bindings)
	return cp, nil
}

func (f *fakeStore) Create(_ context.Context, subject, role, domain string) error {
	f.bindings = append(f.bindings, opabinding.RoleBinding{
		Subject: subject, Role: role, Domain: domain,
	})
	return nil
}

func engine(t *testing.T, bindings ...opabinding.RoleBinding) *Engine {
	t.Helper()
	e, err := newFromStore(context.Background(), &fakeStore{bindings: bindings})
	require.NoError(t, err)
	return e
}

func allow(t *testing.T, e *Engine, subject, domain, resource, action, visibility string) bool {
	t.Helper()
	ok, err := e.Allow(context.Background(), Input{
		Subject:      subject,
		Domain:       domain,
		ResourceType: resource,
		Action:       action,
		Visibility:   visibility,
	})
	require.NoError(t, err)
	return ok
}

// owner role

func TestOwner_CanCreate(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	assert.True(t, allow(t, e, "alice", "alice/mymodule", "module", "create", "private"))
}

func TestOwner_CanTransfer(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "transfer", "private"))
}

func TestOwner_WildcardMatchesAllSubdomains(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	for _, domain := range []string{"alice/bar", "alice/baz", "alice/very-long-name"} {
		assert.True(t, allow(t, e, "alice", domain, "module", "read", "private"), "domain: %s", domain)
	}
}

func TestOwner_WildcardDoesNotMatchOtherNamespace(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	assert.False(t, allow(t, e, "alice", "bob/foo", "module", "read", "private"))
}

// admin role

func TestAdmin_CannotTransfer(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "bob", Role: "admin", Domain: "alice/foo"})
	assert.False(t, allow(t, e, "bob", "alice/foo", "module", "transfer", "private"))
}

func TestAdmin_CanDelete(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "bob", Role: "admin", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "bob", "alice/foo", "module", "delete", "private"))
}

func TestAdmin_CanReadLabels(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "bob", Role: "admin", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "bob", "alice/foo", "label", "read", "private"))
}

// contributor role

func TestContributor_CanPush(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "contributor", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "carol", "alice/foo", "module", "push", "private"))
}

func TestContributor_CannotDelete(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "contributor", Domain: "alice/foo"})
	assert.False(t, allow(t, e, "carol", "alice/foo", "module", "delete", "private"))
}

func TestContributor_CanReadCommits(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "contributor", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "carol", "alice/foo", "commit", "read", "private"))
}

// reader role

func TestReader_CanReadPrivate(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "dave", Role: "reader", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "dave", "alice/foo", "module", "read", "private"))
}

func TestReader_CannotPush(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "dave", Role: "reader", Domain: "alice/foo"})
	assert.False(t, allow(t, e, "dave", "alice/foo", "module", "push", "private"))
}

// public visibility bypass

func TestPublic_ReadableByAnyone(t *testing.T) {
	e := engine(t) // no bindings
	assert.True(t, allow(t, e, "", "alice/foo", "module", "read", "public"))
}

func TestPublic_ListableByAnyone(t *testing.T) {
	e := engine(t)
	assert.True(t, allow(t, e, "", "alice/foo", "module", "list", "public"))
}

func TestPublic_DeleteDeniedWithoutBinding(t *testing.T) {
	e := engine(t)
	assert.False(t, allow(t, e, "anon", "alice/foo", "module", "delete", "public"))
}

// no binding

func TestNoBinding_Denied(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	assert.False(t, allow(t, e, "mallory", "alice/foo", "module", "read", "private"))
}

// exact domain match

func TestExactDomain_MatchesOnly(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "bob", Role: "reader", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "bob", "alice/foo", "module", "read", "private"))
	assert.False(t, allow(t, e, "bob", "alice/bar", "module", "read", "private"))
}

// reload

func TestReload_NewBindingTakesEffect(t *testing.T) {
	store := &fakeStore{}
	ctx := context.Background()
	e, err := newFromStore(ctx, store)
	require.NoError(t, err)

	// Before: denied.
	assert.False(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))

	// Add binding and reload.
	require.NoError(t, e.AddBinding(ctx, "alice", "owner", "alice/*"))

	// After: allowed.
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
}
