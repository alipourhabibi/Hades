package grpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// FromDB maps a storage-layer error to a *connect.Error.
//
// It handles both supported backends: pgx.ErrNoRows (PostgreSQL) and
// sql.ErrNoRows (SQLite) both mean "not found". An error that is already a
// *connect.Error is returned unchanged, so a correctly-coded error raised by a
// lower layer is not flattened to Internal.
//
// Postgres error details are never forwarded to the caller: pgErr.Detail
// embeds the offending column values (for example
// "Key (username)=(alice) already exists"), which is both data disclosure and a
// user-enumeration oracle. Callers are expected to log the raw error.
func FromDB(err error) error {
	if err == nil {
		return nil
	}

	// Already carries an intended status code; pass it through untouched.
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}

	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return connect.NewError(connect.CodeAlreadyExists, errors.New("already exists"))
		case "23503": // foreign_key_violation
			return connect.NewError(connect.CodeFailedPrecondition, errors.New("referenced record does not exist"))
		case "22P02": // invalid_text_representation (e.g. non-UUID in a UUID column)
			return connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
	}

	// SQLite reports constraint violations as a driver error string rather than
	// a typed error, so the message is matched here. The message itself is never
	// returned to the caller.
	switch {
	case isSQLiteUniqueViolation(err):
		return connect.NewError(connect.CodeAlreadyExists, errors.New("already exists"))
	case isSQLiteForeignKeyViolation(err):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("referenced record does not exist"))
	}

	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("request timed out"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

// FromPgx is the previous name of FromDB.
//
// Deprecated: use FromDB. It handles SQLite as well, which is the default
// backend.
func FromPgx(err error) error { return FromDB(err) }

// isSQLiteUniqueViolation reports whether err is a SQLite UNIQUE constraint
// failure. modernc.org/sqlite surfaces these as an untyped error whose message
// contains the constraint kind.
func isSQLiteUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isSQLiteForeignKeyViolation reports whether err is a SQLite FOREIGN KEY
// constraint failure.
func isSQLiteForeignKeyViolation(err error) bool {
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
