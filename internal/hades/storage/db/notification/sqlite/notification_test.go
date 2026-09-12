package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
)

func newNotificationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE notifications (
			id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id     TEXT NOT NULL,
			type        TEXT NOT NULL,
			title       TEXT NOT NULL,
			body        TEXT,
			resource_id TEXT,
			read_at     DATETIME,
			created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
		);
	`)
	require.NoError(t, err)
	return db
}

func TestCreateAndList(t *testing.T) {
	db := newNotificationDB(t)
	store := NewNotification(db)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, "u-alice", notification.TypeCommitPushed, "New commit on alice/mymod", "bob pushed abcdef123456.", "c-1"))
	require.NoError(t, store.Create(ctx, "u-bob", notification.TypeSDKFailed, "go SDK generation failed", "boom", "job-1"))

	alice, err := store.ListForUser(ctx, "u-alice", 50, 0)
	require.NoError(t, err)
	require.Len(t, alice, 1, "a notification is addressed to one user, not broadcast")
	assert.Equal(t, notification.TypeCommitPushed, alice[0].Type)
	assert.Equal(t, "c-1", alice[0].ResourceId)
	assert.False(t, alice[0].Read)

	bob, err := store.ListForUser(ctx, "u-bob", 50, 0)
	require.NoError(t, err)
	require.Len(t, bob, 1)
	assert.Equal(t, notification.TypeSDKFailed, bob[0].Type)
}

func TestMarkReadIsScopedToTheOwner(t *testing.T) {
	db := newNotificationDB(t)
	store := NewNotification(db)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, "u-alice", notification.TypeCommitPushed, "t", "b", "c-1"))
	listed, err := store.ListForUser(ctx, "u-alice", 50, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	id := listed[0].Id

	// Another user naming the same id changes nothing, and the storage layer
	// says so: it returns ErrNotFound rather than reporting success for a
	// statement that matched no row. The handler is where the decision to
	// answer the caller with success is made, so that NotFound never confirms
	// which ids exist; see server/notification.MarkNotificationRead.
	require.ErrorIs(t, store.MarkRead(ctx, id, "u-bob"), notification.ErrNotFound)
	listed, err = store.ListForUser(ctx, "u-alice", 50, 0)
	require.NoError(t, err)
	assert.False(t, listed[0].Read)

	require.NoError(t, store.MarkRead(ctx, id, "u-alice"))
	listed, err = store.ListForUser(ctx, "u-alice", 50, 0)
	require.NoError(t, err)
	assert.True(t, listed[0].Read)
}
