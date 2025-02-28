package authentication

import (
	"context"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/utils/bcrypt"
	"github.com/alipourhabibi/Hades/utils/log"

	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	dbuser "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
)

type Server struct {
	v1connect.AuthenticationServiceHandler

	logger               *log.LoggerWrapper
	userStorage          *dbuser.UserStorage
	sessionStorage       *dbsession.SessionStorage
	authorizationService *authorization.Server
}

func NewServer(
	deps *server.Dependencies,
) *Server {
	return &Server{
		logger:               deps.Logger,
		userStorage:          deps.UserDB,
		sessionStorage:       deps.SessionDB,
		authorizationService: deps.Authorization,
	}
}

func (s *Server) isUserExists(ctx context.Context, username string) (bool, error) {
	_, err := s.userStorage.GetByUsername(ctx, username)
	if err != nil {
		pkgErr := pkgerr.FromPgx(err).(pkgerr.PkgError)
		if pkgErr.Code == pkgerr.NotFound {
			return false, nil
		}
	}
	return true, nil
}

func (s *Server) Signin(ctx context.Context, in *connect.Request[v1.SigninRequest]) (*connect.Response[v1.SigninResponse], error) {
	lg := s.logger.With("method", "Signin")

	exists, err := s.isUserExists(ctx, in.Msg.Username)
	if err != nil {
		lg.Error("failed to get user", "error", err)
		return nil, err
	}

	if exists {
		return nil, pkgerr.New("Username Exists", pkgerr.AlreadyExists)
	}

	hashedPassword, err := bcrypt.HashPassword(in.Msg.Password)
	if err != nil {
		return nil, pkgerr.FromBcrypt(err)
	}

	err = s.userStorage.Create(
		ctx,
		in.Msg.Username,
		in.Msg.Email,
		hashedPassword,
		registryv1.UserType_USER_TYPE_USER,
		registryv1.UserState_USER_STATE_ACTIVE,
		in.Msg.Description,
		"",
	)
	if err != nil {
		return nil, pkgerr.FromPgx(err)
	}

	err = s.authorizationService.AddBasicRoles(ctx, in.Msg.Username)
	if err != nil {
		pkgerr.FromCasbin(err)
		return nil, err
	}

	return &connect.Response[v1.SigninResponse]{
		Msg: &v1.SigninResponse{
			Status: true,
		},
	}, nil

}

func (s *Server) Login(ctx context.Context, in *connect.Request[v1.LoginRequest]) (*connect.Response[v1.LoginResponse], error) {

	resp, err := s.userStorage.GetByUsername(ctx, in.Msg.Username)
	if err != nil {
		return nil, pkgerr.FromPgx(err)
	}

	err = bcrypt.CheckPasswordHash(in.Msg.Password, resp.Password)
	if err != nil {
		return nil, pkgerr.FromBcrypt(err)
	}

	id, err := s.sessionStorage.Create(ctx, resp.Id, "session", time.Now().Add(7*24*time.Hour))
	if err != nil {
		return nil, pkgerr.FromPgx(err)
	}

	return &connect.Response[v1.LoginResponse]{
		Msg: &v1.LoginResponse{
			Token: id,
		},
	}, nil
}
