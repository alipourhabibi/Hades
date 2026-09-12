package authorization

import (
	"context"
	"testing"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore is an in-memory bindingStore used in tests; no database required.
type fakeStore struct {
	bindings         []opabinding.RoleBinding
	listBySubjectHit int // counts ListBySubject calls to verify cache path
}

func (f *fakeStore) Create(_ context.Context, subject, role, domain string) error {
	f.bindings = append(f.bindings, opabinding.RoleBinding{
		Subject: subject, Role: role, Domain: domain,
	})
	return nil
}

func (f *fakeStore) ListBySubject(_ context.Context, subject string) ([]opabinding.RoleBinding, error) {
	f.listBySubjectHit++
	var out []opabinding.RoleBinding
	for _, b := range f.bindings {
		if b.Subject == subject {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteBySubjectDomain(_ context.Context, subject, domain string) error {
	filtered := f.bindings[:0]
	for _, b := range f.bindings {
		if b.Subject != subject || b.Domain != domain {
			filtered = append(filtered, b)
		}
	}
	f.bindings = filtered
	return nil
}

func engine(t *testing.T, bindings ...opabinding.RoleBinding) *Engine {
	t.Helper()
	e, err := newFromStore(context.Background(), &fakeStore{bindings: bindings}, cache.NewMemoryCache(), 0)
	require.NoError(t, err)
	return e
}

// engineWithSuperAdmins builds an engine whose policy bypass list is seeded,
// which is what WithSuperAdmins does in production.
func engineWithSuperAdmins(t *testing.T, admins []string) *Engine {
	t.Helper()
	e, err := newFromStore(context.Background(), &fakeStore{}, cache.NewMemoryCache(), 0,
		WithSuperAdmins(admins))
	require.NoError(t, err)
	return e
}

func allow(t *testing.T, e *Engine, subject, domain, resource, action, visibility string) bool {
	t.Helper()
	ok, err := e.Allow(context.Background(), constants.Policy{
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

func TestAdmin_CanAdministerTheOrg(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "bob", Role: "admin", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "bob", "alice/foo", "org", "admin", "private"))
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

func TestContributor_CanReadTheOrg(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "contributor", Domain: "alice/foo"})
	assert.True(t, allow(t, e, "carol", "alice/foo", "org", "read", "private"))
}

// TestContributor_CannotPublish pins the split between module:update and
// module:publish: pushing commits does not imply the right to publish a
// private module to the world.
func TestContributor_CannotPublish(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "carol", Role: "contributor", Domain: "alice/foo"})
	assert.False(t, allow(t, e, "carol", "alice/foo", "module", "publish", "private"))
}

// TestSuperAdminBypassFires is the regression test for the bypass clause that
// could never fire, because data.superadmins was seeded by nothing.
func TestSuperAdminBypassFires(t *testing.T) {
	e := engineWithSuperAdmins(t, []string{"root"})
	assert.True(t, allow(t, e, "root", "someone/else", "module", "delete", "private"),
		"a configured superadmin bypasses every check")
	assert.False(t, allow(t, e, "mallory", "someone/else", "module", "delete", "private"),
		"anyone else is still refused")
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
	e, err := newFromStore(ctx, store, cache.NewMemoryCache(), 0)
	require.NoError(t, err)

	// Before: denied.
	assert.False(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))

	// Add binding and reload.
	require.NoError(t, e.AddBinding(ctx, "alice", "owner", "alice/*"))

	// After: allowed.
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
}

// TestDeleteBinding_RevokesAccess verifies that DeleteBinding removes access
// immediately by invalidating the cache and removing the DB record.
func TestDeleteBinding_RevokesAccess(t *testing.T) {
	store := &fakeStore{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
	}}
	ctx := context.Background()
	e, err := newFromStore(ctx, store, cache.NewMemoryCache(), 0)
	require.NoError(t, err)

	// Initially allowed.
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))

	// Delete binding.
	require.NoError(t, e.DeleteBinding(ctx, "alice", "alice/*"))

	// Now denied.
	assert.False(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
}

// TestDeleteBinding_OnlyAffectsTargetSubject verifies DeleteBinding for alice
// does not affect bob's bindings.
func TestDeleteBinding_OnlyAffectsTargetSubject(t *testing.T) {
	store := &fakeStore{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
		{Subject: "bob", Role: "reader", Domain: "alice/foo"},
	}}
	ctx := context.Background()
	e, err := newFromStore(ctx, store, cache.NewMemoryCache(), 0)
	require.NoError(t, err)

	require.NoError(t, e.DeleteBinding(ctx, "alice", "alice/*"))

	assert.False(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
	assert.True(t, allow(t, e, "bob", "alice/foo", "module", "read", "private"))
}

// TestAddBinding_CacheInvalidatedOnWrite verifies AddBinding flushes the
// subject's cache so the new binding takes effect on next Allow().
func TestAddBinding_CacheInvalidatedOnWrite(t *testing.T) {
	store := &fakeStore{}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	e, err := newFromStore(ctx, store, c, 60*time.Second)
	require.NoError(t, err)

	// Prime cache with empty result for alice.
	assert.False(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
	assert.Equal(t, 1, store.listBySubjectHit)

	// Add binding; must invalidate cache.
	require.NoError(t, e.AddBinding(ctx, "alice", "owner", "alice/*"))

	// Next Allow must hit DB (cache miss after invalidation).
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
	assert.Equal(t, 2, store.listBySubjectHit, "expected DB hit after cache invalidation from AddBinding")
}

// --- batch ---

func batchAllow(t *testing.T, e *Engine, inputs []constants.Policy) []bool {
	t.Helper()
	results, err := e.BatchAllow(context.Background(), inputs)
	require.NoError(t, err)
	return results
}

func TestBatchAllow_Empty(t *testing.T) {
	e := engine(t)
	results := batchAllow(t, e, nil)
	assert.Empty(t, results)
}

func TestBatchAllow_AllAllowed(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	inputs := []constants.Policy{
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "alice", Domain: "alice/bar", ResourceType: "module", Action: "push", Visibility: "private"},
	}
	results := batchAllow(t, e, inputs)
	assert.Equal(t, []bool{true, true}, results)
}

func TestBatchAllow_FirstDenied(t *testing.T) {
	e := engine(t, opabinding.RoleBinding{Subject: "alice", Role: "owner", Domain: "alice/*"})
	inputs := []constants.Policy{
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "bob", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "alice", Domain: "alice/bar", ResourceType: "module", Action: "push", Visibility: "private"},
	}
	results := batchAllow(t, e, inputs)
	assert.Equal(t, []bool{true, false, true}, results)
}

func TestBatchAllow_AllDenied(t *testing.T) {
	e := engine(t) // no bindings
	inputs := []constants.Policy{
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "bob", Domain: "bob/bar", ResourceType: "module", Action: "push", Visibility: "private"},
	}
	results := batchAllow(t, e, inputs)
	assert.Equal(t, []bool{false, false}, results)
}

func TestBatchAllow_PublicBypass(t *testing.T) {
	e := engine(t) // no bindings
	inputs := []constants.Policy{
		{Subject: "", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "public"},
		{Subject: "", Domain: "alice/bar", ResourceType: "module", Action: "list", Visibility: "public"},
	}
	results := batchAllow(t, e, inputs)
	assert.Equal(t, []bool{true, true}, results)
}

// TestBatchAllow_UniqueSubjectsReduceDBHits verifies that K unique subjects
// → K DB reads regardless of N total policies, because hybridStore caches
// per-subject and OPA only fetches each unique subject path once per eval.
func TestBatchAllow_UniqueSubjectsReduceDBHits(t *testing.T) {
	store := &fakeStore{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
		{Subject: "bob", Role: "reader", Domain: "alice/foo"},
	}}
	e, err := newFromStore(context.Background(), store, cache.NewMemoryCache(), 0)
	require.NoError(t, err)

	// 4 policies, 2 unique subjects
	inputs := []constants.Policy{
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "alice", Domain: "alice/bar", ResourceType: "module", Action: "push", Visibility: "private"},
		{Subject: "bob", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "bob", Domain: "alice/foo", ResourceType: "module", Action: "list", Visibility: "private"},
	}
	results := batchAllow(t, e, inputs)
	assert.Equal(t, []bool{true, true, true, true}, results)
	assert.Equal(t, 2, store.listBySubjectHit, "4 policies with 2 unique subjects must produce exactly 2 DB reads")
}

// TestBatchAllow_SingleEval_MatchesSingleAllow verifies BatchAllow results
// agree with individual Allow() calls for each input.
func TestBatchAllow_SingleEval_MatchesSingleAllow(t *testing.T) {
	bindings := []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
		{Subject: "carol", Role: "contributor", Domain: "alice/foo"},
	}
	e := engine(t, bindings...)
	inputs := []constants.Policy{
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "alice", Domain: "alice/foo", ResourceType: "module", Action: "transfer", Visibility: "private"},
		{Subject: "carol", Domain: "alice/foo", ResourceType: "module", Action: "push", Visibility: "private"},
		{Subject: "carol", Domain: "alice/foo", ResourceType: "module", Action: "delete", Visibility: "private"},
		{Subject: "mallory", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "private"},
		{Subject: "", Domain: "alice/foo", ResourceType: "module", Action: "read", Visibility: "public"},
	}

	batch := batchAllow(t, e, inputs)
	for i, inp := range inputs {
		single := allow(t, e, inp.Subject, inp.Domain, inp.ResourceType, inp.Action, inp.Visibility)
		assert.Equal(t, single, batch[i], "input[%d] mismatch: single=%v batch=%v", i, single, batch[i])
	}
}

// TestCache_HybridStorePopulatedOnAllow verifies that:
//  1. OPA calls hybridStore.Read with path ["role_bindings", subject] (not ["role_bindings"]).
//  2. The result is cached: a second Allow() call does not hit the DB again.
func TestCache_HybridStorePopulatedOnAllow(t *testing.T) {
	store := &fakeStore{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
	}}
	c := cache.NewMemoryCache()
	ctx := context.Background()
	e, err := newFromStore(ctx, store, c, 60*time.Second)
	require.NoError(t, err)

	// First Allow: should hit DB via ListBySubject.
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "read", "private"))
	assert.Equal(t, 1, store.listBySubjectHit, "expected 1 DB hit on first Allow")

	// Cache key must be populated.
	key := bindingCacheKey("alice")
	_, ok, err := c.Get(ctx, key)
	require.NoError(t, err)
	assert.True(t, ok, "cache should be populated after first Allow")

	// Second Allow: should read from cache, not DB.
	assert.True(t, allow(t, e, "alice", "alice/foo", "module", "push", "private"))
	assert.Equal(t, 1, store.listBySubjectHit, "expected no extra DB hit on second Allow (cache hit)")
}
