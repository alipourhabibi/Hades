package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared&_time_format=sqlite")
	require.NoError(t, err)
	_, err = db.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE IF NOT EXISTS resources (id TEXT PRIMARY KEY, resource_type TEXT NOT NULL);`)
	require.NoError(t, err)
	return db
}

func TestSQLiteResourceStorage_RegisterAndResolve(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewResource(db)
	ctx := context.Background()

	tests := []struct {
		id string
		rt resource.ResourceType
	}{
		{"abc-def-123", resource.ResourceTypeModule},
		{"deadbeef-0000-0000-0000-000000000001", resource.ResourceTypeCommit},
		{"label-uuid-0000-0000-0000-000000000002", resource.ResourceTypeLabel},
	}

	for _, tc := range tests {
		require.NoError(t, s.Register(ctx, tc.id, tc.rt))
		got, err := s.ResolveType(ctx, tc.id)
		require.NoError(t, err)
		assert.Equal(t, tc.rt, got)
	}
}

func TestSQLiteResourceStorage_DashlessUUID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewResource(db)
	ctx := context.Background()

	dashed := "deadbeef-dead-dead-dead-deadbeef0001"
	dashless := "deadbeefdeaddeaddeaddeadbeef0001"

	require.NoError(t, s.Register(ctx, dashed, resource.ResourceTypeCommit))

	// Resolve using dashless form (as buf CLI sends).
	got, err := s.ResolveType(ctx, dashless)
	require.NoError(t, err)
	assert.Equal(t, resource.ResourceTypeCommit, got)
}

func TestSQLiteResourceStorage_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewResource(db)
	ctx := context.Background()

	_, err := s.ResolveType(ctx, "no-such-id")
	require.Error(t, err)
}

func TestSQLiteResourceStorage_DuplicateRegisterIdempotent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewResource(db)
	ctx := context.Background()

	id := "some-module-id"
	require.NoError(t, s.Register(ctx, id, resource.ResourceTypeModule))
	require.NoError(t, s.Register(ctx, id, resource.ResourceTypeModule), "duplicate register must be idempotent")
}
