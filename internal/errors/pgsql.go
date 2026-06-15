package errors

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// FromPgx maps a pgx error to a PkgError with the appropriate status code.
func FromPgx(err error) error {
	if err == nil {
		return nil
	}

	// already a package error; keep as-is
	var pkgErr *PkgError
	if errors.As(err, &pkgErr) {
		return err
	}

	switch {
	case IsQueryCancelled(err):
		return New(err.Error(), Canceled)

	case IsNotFound(err):
		return New("not found", NotFound)

	case IsUniqueViolation(err):
		return New("already exists", AlreadyExists)

	default:
		return New("unknown error: "+err.Error(), Unknown)
	}
}

// IsNotFound reports whether err indicates that no rows were returned.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// IsQueryCancelled reports whether err is a PostgreSQL query_canceled (57014).
func IsQueryCancelled(err error) bool {
	return isPgError(err, "57014")
}

// IsUniqueViolation reports whether err is a PostgreSQL unique_violation (23505).
func IsUniqueViolation(err error) bool {
	return isPgError(err, "23505")
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
