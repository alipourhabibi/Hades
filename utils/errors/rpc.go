package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
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
func ResourceExhausted(msg string) error {
	return connect.NewError(connect.CodeResourceExhausted, errors.New(msg))
}
func Unknown(msg string) error { return connect.NewError(connect.CodeUnknown, errors.New(msg)) }

// Internal returns a CodeInternal error. The message argument is intentionally
// ignored; a generic message is returned to avoid leaking internal details.
func Internal(msg string) error {
	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

// ToConnectError converts a PkgError to a *connect.Error. Errors that are
// already *connect.Error are returned as-is.
func ToConnectError(err error) error {
	if err == nil {
		return err
	}

	// Do not double-wrap errors that are already *connect.Error.
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}

	if se, ok := err.(pkgerr.PkgError); ok {
		switch se.Code {
		case pkgerr.Code(codes.AlreadyExists):
			err = connect.NewError(connect.CodeAlreadyExists, errors.New(se.Message))
		case pkgerr.Code(codes.Canceled):
			err = connect.NewError(connect.CodeCanceled, errors.New(se.Message))
		case pkgerr.Code(codes.Unknown):
			err = connect.NewError(connect.CodeUnknown, errors.New(se.Message))
		case pkgerr.Code(codes.InvalidArgument):
			err = connect.NewError(connect.CodeInvalidArgument, errors.New(se.Message))
		case pkgerr.Code(codes.DeadlineExceeded):
			err = connect.NewError(connect.CodeDeadlineExceeded, errors.New(se.Message))
		case pkgerr.Code(codes.NotFound):
			err = connect.NewError(connect.CodeNotFound, errors.New(se.Message))
		case pkgerr.Code(codes.PermissionDenied):
			err = connect.NewError(connect.CodePermissionDenied, errors.New(se.Message))
		case pkgerr.Code(codes.ResourceExhausted):
			err = connect.NewError(connect.CodeResourceExhausted, errors.New(se.Message))
		case pkgerr.Code(codes.FailedPrecondition):
			err = connect.NewError(connect.CodeFailedPrecondition, errors.New(se.Message))
		case pkgerr.Code(codes.Aborted):
			err = connect.NewError(connect.CodeAborted, errors.New(se.Message))
		case pkgerr.Code(codes.OutOfRange):
			err = connect.NewError(connect.CodeOutOfRange, errors.New(se.Message))
		case pkgerr.Code(codes.Unimplemented):
			err = connect.NewError(connect.CodeUnimplemented, errors.New(se.Message))
		case pkgerr.Code(codes.Internal):
			err = connect.NewError(connect.CodeInternal, errors.New("internal server error"))
		case pkgerr.Code(codes.Unavailable):
			err = connect.NewError(connect.CodeUnavailable, errors.New(se.Message))
		case pkgerr.Code(codes.DataLoss):
			err = connect.NewError(connect.CodeDataLoss, errors.New(se.Message))
		case pkgerr.Code(codes.Unauthenticated):
			err = connect.NewError(connect.CodeUnauthenticated, errors.New(se.Message))
		default:
			err = connect.NewError(connect.CodeUnknown, errors.New(se.Message))
		}
	}

	return err
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
