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

func toConnectError(err error) error {
	if err == nil {
		return err
	}

	if se, ok := err.(pkgerr.PkgError); ok {
		switch se.Code {
		case pkgerr.Code(codes.AlreadyExists):
			err = connect.NewError(connect.CodeAlreadyExists, errors.New(se.Message))
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
