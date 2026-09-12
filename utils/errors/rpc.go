package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/grpc/status"
)

// Convenience constructors for common Connect error codes.

func NotFound(msg string) error { return connect.NewError(connect.CodeNotFound, errors.New(msg)) }
func InvalidArgument(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}
func AlreadyExists(msg string) error {
	return connect.NewError(connect.CodeAlreadyExists, errors.New(msg))
}
func PermissionDenied(msg string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New(msg))
}
func Unauthenticated(msg string) error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New(msg))
}
func Unimplemented(msg string) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New(msg))
}
func Unavailable(msg string) error { return connect.NewError(connect.CodeUnavailable, errors.New(msg)) }
func FailedPrecondition(msg string) error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
}
func ResourceExhausted(msg string) error {
	return connect.NewError(connect.CodeResourceExhausted, errors.New(msg))
}
func Unknown(msg string) error { return connect.NewError(connect.CodeUnknown, errors.New(msg)) }

// Internal returns a CodeInternal error. The message argument is intentionally
// ignored; a generic message is returned to avoid leaking internal details.
func Internal(msg string) error {
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

// ToConnectError ensures err is a *connect.Error. Errors already of that type
// are returned as-is; all others are wrapped as CodeInternal so internal
// details are never leaked to callers.
func ToConnectError(err error) error {
	if err == nil {
		return err
	}

	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}

	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

// UnwrapGRPCStatus recursively unwraps err looking for a gRPC status.
func UnwrapGRPCStatus(err error) *status.Status {
	if se, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
		return se.GRPCStatus()
	}
	e := errors.Unwrap(err)
	if e == nil {
		return nil
	}
	return UnwrapGRPCStatus(e)
}

// NewErrorInterceptor returns a Connect interceptor that translates PkgError
// values returned by handlers into proper Connect error codes.
func NewErrorInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			return resp, ToConnectError(err)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
