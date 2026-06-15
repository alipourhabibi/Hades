package middleware

import (
	"context"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	connErr "github.com/alipourhabibi/Hades/utils/errors"
)

// NewProtovalidateInterceptor returns a Connect unary interceptor that validates
// every inbound request message against its buf.validate constraints.
// Returns CodeInvalidArgument if validation fails.
func NewProtovalidateInterceptor() (connect.Interceptor, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, err
	}
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if msg, ok := req.Any().(proto.Message); ok {
				if vErr := v.Validate(msg); vErr != nil {
					return nil, connErr.InvalidArgument(vErr.Error())
				}
			}
			return next(ctx, req)
		}
	}), nil
}
