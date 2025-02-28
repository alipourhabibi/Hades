package authorization

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	utilserr "github.com/alipourhabibi/Hades/utils/errors"
)

func (s *Server) NewAuthorizationInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {

			authHeader := req.Header().Get("Authorization")
			if authHeader == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("Authorization header is required"))
			}

			// Split the header into parts
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("Authorization header is invalid"))
			}

			user, err := s.authService.UserBySession(ctx, parts[1])
			if err != nil {
				err = utilserr.ToConnectError(err)
				return nil, err
			}

			ctx = context.WithValue(ctx, "user", user)
			ctx = context.WithValue(ctx, "Authorization", parts[1])

			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
