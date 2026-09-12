package connerr

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	sqlite "modernc.org/sqlite"
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

	// ErrNotFound covers the storage-package sentinels, which wrap it, as well
	// as the two driver forms. Matching only pgx.ErrNoRows, which is what the
	// deleted MapUserAuthError did, meant a missing row on SQLite mapped to
	// Internal while the same miss on PostgreSQL mapped correctly.
	if errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
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

	// SQLite constraint violations, classified by result code. The driver's
	// message is never returned to the caller.
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

// SQLite extended result codes.
//
// modernc.org/sqlite turns extended result codes on when it opens a connection,
// so *sqlite.Error carries one of these rather than the plain SQLITE_CONSTRAINT
// (19) they refine. They are spelled out here rather than imported from
// modernc.org/sqlite/lib, which is a very large generated package to pull in
// for four integers.
const (
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintRowID      = 2579 // SQLITE_CONSTRAINT_ROWID
)

// sqliteErrorCode returns the SQLite result code carried by err.
//
// This used to be a substring test on err.Error(), under a comment claiming
// modernc.org/sqlite returns an untyped error. It does not: it returns
// *sqlite.Error, with the result code available through Code(), and the type
// survives database/sql. Two things followed from matching the prose instead.
// A duplicate primary key says "PRIMARY KEY constraint failed", not "UNIQUE
// constraint failed", so inserting a row whose id already existed was reported
// to the client as Internal rather than AlreadyExists. And the test went the
// other way: any error whose message merely contained the phrase was mapped to
// AlreadyExists, including one that had picked the text up from a lower layer
// or from caller-supplied input.
func sqliteErrorCode(err error) (int, bool) {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return 0, false
	}
	return se.Code(), true
}

// isSQLiteUniqueViolation reports whether err is SQLite refusing a row because
// one with that key is already there. All three spellings of that mean
// AlreadyExists to a caller.
func isSQLiteUniqueViolation(err error) bool {
	code, ok := sqliteErrorCode(err)
	if !ok {
		return false
	}
	return code == sqliteConstraintUnique ||
		code == sqliteConstraintPrimaryKey ||
		code == sqliteConstraintRowID
}

// isSQLiteForeignKeyViolation reports whether err is a SQLite FOREIGN KEY
// constraint failure.
func isSQLiteForeignKeyViolation(err error) bool {
	code, ok := sqliteErrorCode(err)
	return ok && code == sqliteConstraintForeignKey
}
