package connerr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestFromDB_NotFoundOnBothBackends(t *testing.T) {
	// SQLite is the default backend, so sql.ErrNoRows must map to NotFound.
	// Mapping it to Internal turned every missing row into a 500.
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(FromDB(sql.ErrNoRows)))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(FromDB(pgx.ErrNoRows)))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(FromDB(fmt.Errorf("wrapped: %w", sql.ErrNoRows))))
}

func TestFromDB_PassesThroughConnectErrors(t *testing.T) {
	// A lower layer that already chose a code must not have it flattened.
	original := connect.NewError(connect.CodePermissionDenied, errors.New("nope"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(FromDB(original)))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(FromDB(fmt.Errorf("wrapped: %w", original))))
}

func TestFromDB_DoesNotLeakPostgresDetail(t *testing.T) {
	// pgErr.Detail embeds the offending column values, which is both data
	// disclosure and a user-enumeration oracle.
	pgErr := &pgconn.PgError{
		Code:    "23505",
		Detail:  "Key (username)=(alice) already exists.",
		Message: "duplicate key value violates unique constraint",
	}
	err := FromDB(pgErr)

	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "alice")
	assert.NotContains(t, err.Error(), "username")
}

func TestFromDB_ForeignKeyViolationDetailNotLeaked(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:   "23503",
		Detail: "Key (module_id)=(deadbeef) is not present in table \"modules\".",
	}
	err := FromDB(pgErr)

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "deadbeef")
}

// The constraint errors come from a real database rather than from a
// hand-written string. *sqlite.Error has unexported fields and no constructor,
// so a driver error is the only way to get one, and a test that fabricates the
// message instead is testing the fabrication: the previous version of this test
// passed against a substring match that mapped a duplicate primary key to
// Internal, because it never produced one.
func TestFromDB_SQLiteConstraintViolations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE parent (id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES parent(id))`)
	require.NoError(t, err)
	_, err = db.Exec(`PRAGMA foreign_keys = ON`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO parent (id, name) VALUES ('1', 'a')`)
	require.NoError(t, err)

	_, uniqueErr := db.Exec(`INSERT INTO parent (id, name) VALUES ('2', 'a')`)
	require.Error(t, uniqueErr)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(FromDB(uniqueErr)))

	// A duplicate primary key says "PRIMARY KEY constraint failed", not
	// "UNIQUE constraint failed", and means the same thing to a caller.
	_, pkErr := db.Exec(`INSERT INTO parent (id, name) VALUES ('1', 'b')`)
	require.Error(t, pkErr)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(FromDB(pkErr)))

	_, fkErr := db.Exec(`INSERT INTO child (id, parent_id) VALUES ('1', 'nosuch')`)
	require.Error(t, fkErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(FromDB(fkErr)))

	// And an ordinary error that merely carries the words is not a constraint
	// violation. Under the substring match it was.
	impostor := errors.New(`upstream said: UNIQUE constraint failed: users.username`)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(FromDB(impostor)))
}

func TestFromDB_ContextErrors(t *testing.T) {
	assert.Equal(t, connect.CodeCanceled, connect.CodeOf(FromDB(context.Canceled)))
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(FromDB(context.DeadlineExceeded)))
}

func TestFromDB_UnknownErrorIsGenericInternal(t *testing.T) {
	err := FromDB(errors.New("connection to host 10.0.0.5 refused, password=hunter2"))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "hunter2")
	assert.NotContains(t, err.Error(), "10.0.0.5")
}

func TestFromDB_NilIsNil(t *testing.T) {
	assert.NoError(t, FromDB(nil))
}
