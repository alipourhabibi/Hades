package errors

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func FromPgx(err error) error {
	if IsQueryCancelled(err) {
		return New(err.Error(), Canceled)
	} else if IsNotFound(err) {
		return New(err.Error(), NotFound)
	} else if IsUniqueViolation(err) {
		return New(err.Error(), AlreadyExists)
	} else {
		return New("unknown error: "+err.Error(), Unknown)
	}
}

// IsNotFound reutrns true if not rows found
func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

// IsQueryCancelled returns true if an error is a query cancellation.
func IsQueryCancelled(err error) bool {
	// https://www.postgresql.org/docs/11/errcodes-appendix.html
	// query_canceled
	return isPgError(err, "57014")
}

// IsUniqueViolation returns true if an error is a unique violation.
func IsUniqueViolation(err error) bool {
	// https://www.postgresql.org/docs/11/errcodes-appendix.html
	// unique_violation
	return isPgError(err, "23505")
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return true
	}
	return false
}
