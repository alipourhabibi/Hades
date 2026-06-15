// Package bufauthn implements buf.alpha.registry.v1alpha1.AuthnService.
// It adapts the buf wire types to internal types and delegates identity
// resolution to the authentication middleware (user already in context).
package bufauthn

import (
	"context"
	"errors"

	registryv1alpha1connect "buf.build/gen/go/bufbuild/buf/connectrpc/go/buf/alpha/registry/v1alpha1/registryv1alpha1connect"
	v1alpha1 "buf.build/gen/go/bufbuild/buf/protocolbuffers/go/buf/alpha/registry/v1alpha1"
	"connectrpc.com/connect"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Server is the buf.alpha.registry.v1alpha1.AuthnService adapter.
// The auth middleware validates the token and sets the user in context
// before this handler runs; GetCurrentUser just reads it back out.
type Server struct {
	registryv1alpha1connect.AuthnServiceHandler

	logger *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger: deps.Logger,
	}
}

func (s *Server) GetCurrentUser(
	ctx context.Context,
	_ *connect.Request[v1alpha1.GetCurrentUserRequest],
) (*connect.Response[v1alpha1.GetCurrentUserResponse], error) {
	u, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok || u == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}

	user := v1alpha1.User_builder{
		Id:       u.GetId(),
		Username: u.GetUsername(),
	}.Build()

	return connect.NewResponse(
		v1alpha1.GetCurrentUserResponse_builder{User: user}.Build(),
	), nil
}
