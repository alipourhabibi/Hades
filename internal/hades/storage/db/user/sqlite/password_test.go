package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// identityv1.User is returned straight to callers by GetUser, ListUsers,
// UserBySession and every response that embeds an owner record. A password
// hash loaded into User.Password therefore leaves the server, so the read
// queries must not select it at all. These tests pin that.

func newUserDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE users (
			id                 TEXT PRIMARY KEY,
			create_time        DATETIME NOT NULL DEFAULT (datetime('now')),
			update_time        DATETIME NOT NULL DEFAULT (datetime('now')),
			username           TEXT NOT NULL,
			email              TEXT NOT NULL DEFAULT '',
			password           TEXT NOT NULL DEFAULT '',
			type               INTEGER NOT NULL,
			state              INTEGER NOT NULL DEFAULT 1,
			description        TEXT NOT NULL DEFAULT '',
			url                TEXT NOT NULL DEFAULT '',
			email_verified_at  DATETIME,
			failed_login_count INTEGER NOT NULL DEFAULT 0,
			locked_until       DATETIME
		);
		INSERT INTO users (id, username, email, password, type) VALUES
			('u-alice', 'alice', 'alice@example.com', '$2a$10$notarealhashbutlongenough', 2);
	`)
	require.NoError(t, err)
	return db
}

func TestUserReadsNeverCarryPasswordHash(t *testing.T) {
	db := newUserDB(t)
	store := NewUser(db)
	ctx := context.Background()

	byName, err := store.GetByUsername(ctx, "alice")
	require.NoError(t, err)
	assert.Empty(t, byName.Password, "GetByUsername must not carry the password hash")

	byID, err := store.GetByID(ctx, "u-alice")
	require.NoError(t, err)
	assert.Empty(t, byID.Password, "GetByID must not carry the password hash")

	byEmail, err := store.GetByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.Empty(t, byEmail.Password, "GetByEmail must not carry the password hash")

	listed, err := store.List(ctx, "")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Empty(t, listed[0].Password, "List must not carry the password hash")

	updated, err := store.Update(ctx, "u-alice", "desc", "https://example.com")
	require.NoError(t, err)
	assert.Empty(t, updated.Password, "Update must not carry the password hash")
}

func TestAuthFieldsStillCarryPasswordHash(t *testing.T) {
	// The hash is still reachable where password checks happen. Dropping it
	// from the user reads must not disarm login.
	db := newUserDB(t)
	store := NewUser(db)
	ctx := context.Background()

	af, err := store.GetAuthFieldsByUsername(ctx, "alice")
	require.NoError(t, err)
	assert.NotEmpty(t, af.PasswordHash)

	afByID, err := store.GetAuthFieldsByID(ctx, "u-alice")
	require.NoError(t, err)
	assert.NotEmpty(t, afByID.PasswordHash)
}
