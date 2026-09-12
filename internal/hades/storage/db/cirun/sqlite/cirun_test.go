package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newCIRunDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE ci_runs (
			id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			module_id       TEXT NOT NULL,
			commit_hash     TEXT NOT NULL,
			lint_passed     INTEGER NOT NULL DEFAULT 0,
			breaking_passed INTEGER NOT NULL DEFAULT 0,
			lint_errors     TEXT NOT NULL DEFAULT '[]',
			breaking_errors TEXT NOT NULL DEFAULT '[]',
			created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
			UNIQUE(module_id, commit_hash)
		);
	`)
	require.NoError(t, err)
	return db
}

func TestCreateThenGet(t *testing.T) {
	db := newCIRunDB(t)
	store := NewCIRun(db)
	ctx := context.Background()

	created, err := store.Create(ctx, "m-1", "abcdef123456", true, true, nil, nil)
	require.NoError(t, err)
	assert.True(t, created.LintPassed)
	assert.True(t, created.BreakingPassed)

	got, err := store.GetByModuleAndCommit(ctx, "m-1", "abcdef123456")
	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
	assert.Empty(t, got.LintErrors)
	assert.Empty(t, got.BreakingErrors)
}

func TestCreateIsIdempotentPerCommit(t *testing.T) {
	// The upload path writes the record inside the commit transaction. A retry
	// of the same push must update the row rather than trip the unique index
	// and fail an otherwise valid push.
	db := newCIRunDB(t)
	store := NewCIRun(db)
	ctx := context.Background()

	_, err := store.Create(ctx, "m-1", "abcdef123456", false, false, []string{"old lint error"}, nil)
	require.NoError(t, err)

	updated, err := store.Create(ctx, "m-1", "abcdef123456", true, true, nil, nil)
	require.NoError(t, err)
	assert.True(t, updated.LintPassed)
	assert.Empty(t, updated.LintErrors, "the second write replaces the first result rather than merging")

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ci_runs`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestGetIsScopedToTheModule(t *testing.T) {
	// Commit hashes are unique registry-wide, but the lookup is still keyed by
	// module so a record cannot be read through a module the caller was not
	// authorised for.
	db := newCIRunDB(t)
	store := NewCIRun(db)
	ctx := context.Background()

	_, err := store.Create(ctx, "m-1", "abcdef123456", true, true, nil, nil)
	require.NoError(t, err)

	_, err = store.GetByModuleAndCommit(ctx, "m-2", "abcdef123456")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
