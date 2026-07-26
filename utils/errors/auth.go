package grpc

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
)

// MapUserAuthError converts a storage error from user lookup into the correct
// RPC error code. ErrNoRows (session or user not found) maps to Unauthenticated;
// all other errors map to Internal so storage details are never leaked.
func MapUserAuthError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}
