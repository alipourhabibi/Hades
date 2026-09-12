package grpc

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

func TestFromDB_SQLiteConstraintViolations(t *testing.T) {
	unique := errors.New("constraint failed: UNIQUE constraint failed: users.username (2067)")
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(FromDB(unique)))

	fk := errors.New("constraint failed: FOREIGN KEY constraint failed (787)")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(FromDB(fk)))
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
