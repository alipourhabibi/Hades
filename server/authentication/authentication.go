package authentication

import (
	"context"

	"connectrpc.com/connect"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/pkg/services/authentication"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	v1connect.AuthenticationServiceHandler

	logger      *log.LoggerWrapper
	authService *authentication.Service
}

func NewServer(l *log.LoggerWrapper, authService *authentication.Service) *Server {
	return &Server{
		logger:      l,
		authService: authService,
	}
}

func (s *Server) Signin(ctx context.Context, in *connect.Request[v1.SigninRequest]) (*connect.Response[v1.SigninResponse], error) {
	user, err := s.authService.Signin(ctx, &models.SigninRequest{
		Username:    in.Msg.Username,
		Password:    in.Msg.Password,
		Description: in.Msg.Description,
	})
	if err != nil {
		return nil, err
	}

	userResponse, err := models.ToRegistryPbV1(user.User)
	if err != nil {
		return nil, err
	}

	return &connect.Response[v1.SigninResponse]{
		Msg: &v1.SigninResponse{
			User: userResponse,
		},
	}, nil
}
