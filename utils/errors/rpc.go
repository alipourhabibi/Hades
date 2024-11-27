package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
)

// TODO add error wrapper here
func toGRPCError(err error) error {
	if err == nil {
		return err
	}

	if se, ok := err.(pkgerr.PkgError); ok {
		switch se.Code {
		case pkgerr.Code(codes.AlreadyExists):
			err = status.Error(codes.AlreadyExists, se.Message)
		}
	}

	return err
}

// TODO thing about returning the error as it may create security errors
func toConnectError(err error) error {
	if err == nil {
		return err
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
			err = connect.NewError(connect.CodeInternal, errors.New(se.Message))
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

// NewErrorInterceptor for wrapping return error
func NewErrorInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			return resp, toConnectError(err)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
