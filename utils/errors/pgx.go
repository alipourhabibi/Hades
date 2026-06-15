package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// FromPgx maps pgx/postgres errors directly to *connect.Error.
// It mirrors internal/errors/pgsql.go but returns connect-native errors so
// handlers do not need to route through the PkgError intermediate type.
func FromPgx(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return connect.NewError(connect.CodeAlreadyExists, errors.New(pgErr.Detail))
		case "23503": // foreign_key_violation
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(pgErr.Detail))
		}
	}
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}
