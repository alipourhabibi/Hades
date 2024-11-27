package authorization

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/alipourhabibi/Hades/api/gen/api/authorization/v1"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	"github.com/alipourhabibi/Hades/pkg/services/authorization"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	authorizationv1connect.AuthorizationHandler

	logger      *log.LoggerWrapper
	authService *authorization.Service
}

func NewServer(l *log.LoggerWrapper, authService *authorization.Service) *Server {
	return &Server{
		logger:      l,
		authService: authService,
	}
}

func (s *Server) UserBySession(ctx context.Context, in *connect.Request[v1.UserBySessionRequest]) (*connect.Response[v1.UserBySessionResponse], error) {
	sessinId := in.Header().Get("User-Session")
	if sessinId == "" {
		return nil, pkgerr.New("SessionId required", pkgerr.Unauthenticated)
	}
	user, err := s.authService.UserBySession(ctx, sessinId)
	if err != nil {
		return nil, err
	}

	pbUser, err := models.ToRegistryPbV1(user)
	if err != nil {
		return nil, err
	}

	return &connect.Response[v1.UserBySessionResponse]{
		Msg: &v1.UserBySessionResponse{
			User: pbUser,
		},
	}, nil
}
