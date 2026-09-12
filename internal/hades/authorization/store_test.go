package authorization

import (
	"context"
	"testing"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hybridStoreDB is a minimal in-memory bindingStore for hybridStore tests.
type hybridStoreDB struct {
	bindings []opabinding.RoleBinding
	hits     int
}

func (d *hybridStoreDB) ListBySubject(_ context.Context, subject string) ([]opabinding.RoleBinding, error) {
	d.hits++
	var out []opabinding.RoleBinding
	for _, b := range d.bindings {
		if b.Subject == subject {
			out = append(out, b)
		}
	}
	return out, nil
}

func newStore(t *testing.T, db hybridBindingDB, c cache.Cache, ttl time.Duration) *hybridStore {
	t.Helper()
	s, err := newHybridStore(c, db, ttl, nil)
	require.NoError(t, err)
	return s
}

// TestHybridStore_ReadFromDB verifies a cache-miss causes ListBySubject to be called.
func TestHybridStore_ReadFromDB(t *testing.T) {
	db := &hybridStoreDB{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
	}}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	val, err := s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)

	slice, ok := val.([]any)
	require.True(t, ok)
	assert.Len(t, slice, 1)
	assert.Equal(t, 1, db.hits)
}

// TestHybridStore_ReadCacheHit verifies second Read uses cache, not DB.
func TestHybridStore_ReadCacheHit(t *testing.T) {
	db := &hybridStoreDB{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
	}}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	assert.Equal(t, 1, db.hits)

	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	assert.Equal(t, 1, db.hits, "second read must hit cache, not DB")
}

// TestHybridStore_WriteInvalidatesCache verifies Write clears the cache entry.
func TestHybridStore_WriteInvalidatesCache(t *testing.T) {
	db := &hybridStoreDB{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
	}}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	// Populate cache.
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	assert.Equal(t, 1, db.hits)

	// Write (invalidate) alice's cache.
	err = s.Write(ctx, txn, storage.AddOp, storage.MustParsePath("/role_bindings/alice"), nil)
	require.NoError(t, err)

	// Read again: cache miss, DB hit.
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	assert.Equal(t, 2, db.hits, "cache was invalidated; DB should be hit again")
}

// TestHybridStore_WriteOnlyInvalidatesTargetSubject verifies other subjects' caches survive.
func TestHybridStore_WriteOnlyInvalidatesTargetSubject(t *testing.T) {
	db := &hybridStoreDB{bindings: []opabinding.RoleBinding{
		{Subject: "alice", Role: "owner", Domain: "alice/*"},
		{Subject: "bob", Role: "reader", Domain: "alice/foo"},
	}}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	// Populate both subjects.
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/bob"))
	require.NoError(t, err)
	assert.Equal(t, 2, db.hits)

	// Invalidate alice only.
	err = s.Write(ctx, txn, storage.AddOp, storage.MustParsePath("/role_bindings/alice"), nil)
	require.NoError(t, err)

	// Bob's read should still be cached.
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/bob"))
	require.NoError(t, err)
	assert.Equal(t, 2, db.hits, "bob's cache must survive alice's invalidation")

	// Alice's read must hit DB.
	_, err = s.Read(ctx, txn, storage.MustParsePath("/role_bindings/alice"))
	require.NoError(t, err)
	assert.Equal(t, 3, db.hits)
}

// TestHybridStore_ReadEmptySubject returns empty slice (not error) for unknown subject.
func TestHybridStore_ReadEmptySubject(t *testing.T) {
	db := &hybridStoreDB{}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	val, err := s.Read(ctx, txn, storage.MustParsePath("/role_bindings/nobody"))
	require.NoError(t, err)

	slice, ok := val.([]any)
	require.True(t, ok)
	assert.Empty(t, slice)
}

// TestHybridStore_NonBindingPathDelegatesToInner verifies non-role_bindings paths
// are served by the inner store (which holds the Rego policy).
func TestHybridStore_NonBindingPathDelegatesToInner(t *testing.T) {
	db := &hybridStoreDB{}
	c := cache.NewMemoryCache()
	s := newStore(t, db, c, 60*time.Second)

	ctx := context.Background()
	txn, err := s.NewTransaction(ctx)
	require.NoError(t, err)

	// Reading /role_bindings (root, no subject) should NOT trigger DB.
	// It falls through to inner; inner has empty map, so we get back the map or error.
	// The important thing is DB.hits stays 0.
	_, _ = s.Read(ctx, txn, storage.MustParsePath("/role_bindings"))
	assert.Equal(t, 0, db.hits, "reading /role_bindings root must not call DB.ListBySubject")
}
