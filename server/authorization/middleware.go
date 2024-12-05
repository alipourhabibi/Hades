package authorization

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	utilserr "github.com/alipourhabibi/Hades/utils/errors"
)

func (s *Server) NewAuthorizationInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {

			userSession := req.Header().Get("User-Session")
			if userSession == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("User-Session required"))
			}

			user, err := s.authService.UserBySession(ctx, userSession)
			if err != nil {
				err = utilserr.ToConnectError(err)
				return nil, err
			}

			ctx = context.WithValue(ctx, "user", user)
			ctx = context.WithValue(ctx, "User-Session", userSession)

			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
