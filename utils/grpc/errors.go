package grpc

import (
	"context"

	"connectrpc.com/connect"
)

// TODO add error wrapper here
func toGRPCError(err error) error {
	return err
}

// NewErrorInterceptor for wrapping return error
func NewErrorInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			return resp, toGRPCError(err)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
