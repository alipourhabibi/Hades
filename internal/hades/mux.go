package hades

import (
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/otel"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/server/middleware"
	errorsutils "github.com/alipourhabibi/Hades/utils/errors"
)

// newServerMux registers all Connect-RPC and gRPC reflection handlers.
func (s *SchemaRegistryServer) newServerMux() (*http.ServeMux, error) {
	protovalidateInterceptor, err := middleware.NewProtovalidateInterceptor()
	if err != nil {
		return nil, fmt.Errorf("protovalidate interceptor: %w", err)
	}

	otelInterceptor, _ := otelconnect.NewInterceptor(
		otelconnect.WithTracerProvider(otel.GetTracerProvider()),
		otelconnect.WithMeterProvider(otel.GetMeterProvider()),
	)

	base := []connect.Interceptor{
		protovalidateInterceptor,
		otelInterceptor,
		errorsutils.NewErrorInterceptor(),
	}

	withAuth := connect.WithInterceptors(append(
		[]connect.Interceptor{s.serverSet.AuthorizationServer.NewAuthorizationInterceptor()},
		base...,
	)...)
	noAuth := connect.WithInterceptors(base...)

	reflector := grpcreflect.NewStaticReflector(
		authenticationv1connect.AuthenticationServiceName,
		authenticationv1connect.SessionServiceName,
		authenticationv1connect.OAuthServiceName,
		authenticationv1connect.APITokenServiceName,
		authenticationv1connect.DeviceServiceName,
		authenticationv1connect.TOTPServiceName,
		authenticationv1connect.AuditServiceName,
		authorizationv1connect.AuthorizationName,
		registryv1connect.ModuleServiceName,
		registryv1connect.CommitServiceName,
		registryv1connect.DiffServiceName,
		registryv1connect.UserServiceName,
		registryv1connect.SDKServiceName,
		registryv1connect.OrgServiceName,
		registryv1connect.CIServiceName,
		registryv1connect.NotificationServiceName,
		registryv1connect.TreeServiceName,
	)

	mux := http.NewServeMux()

	mux.Handle(authenticationv1connect.NewAuthenticationServiceHandler(s.serverSet.AuthenticationServer, withAuth))
	mux.Handle(authorizationv1connect.NewAuthorizationHandler(s.serverSet.AuthorizationServer, withAuth))
	mux.Handle(registryv1connect.NewModuleServiceHandler(s.serverSet.ModuleServer, withAuth))
	mux.Handle(modulev1connect.NewModuleServiceHandler(s.serverSet.BufModuleServer, withAuth))
	mux.Handle(modulev1connect.NewCommitServiceHandler(s.serverSet.BufCommitServer, withAuth))
	mux.Handle(modulev1connect.NewUploadServiceHandler(s.serverSet.BufUploadServer, withAuth))
	mux.Handle(modulev1connect.NewGraphServiceHandler(s.serverSet.BufGraphServer, withAuth))
	mux.Handle(modulev1connect.NewDownloadServiceHandler(s.serverSet.BufDownloadServer, withAuth))
	mux.Handle(authenticationv1connect.NewSessionServiceHandler(s.serverSet.SessionHandler, withAuth))
	mux.Handle(authenticationv1connect.NewOAuthServiceHandler(s.serverSet.OAuthHandler, noAuth))
	mux.Handle(authenticationv1connect.NewAPITokenServiceHandler(s.serverSet.APITokenHandler, withAuth))
	mux.Handle(authenticationv1connect.NewDeviceServiceHandler(s.serverSet.DeviceHandler, noAuth))
	mux.Handle(authenticationv1connect.NewTOTPServiceHandler(s.serverSet.TOTPHandler, withAuth))
	mux.Handle(authenticationv1connect.NewAuditServiceHandler(s.serverSet.AuditHandler, withAuth))
	mux.Handle(registryv1connect.NewCommitServiceHandler(s.serverSet.CommitHandler, withAuth))
	mux.Handle(registryv1connect.NewDiffServiceHandler(s.serverSet.DiffHandler, withAuth))
	mux.Handle(registryv1connect.NewUserServiceHandler(s.serverSet.UserHandler, withAuth))
	mux.Handle(registryv1connect.NewSDKServiceHandler(s.serverSet.SDKHandler, withAuth))
	mux.Handle(registryv1connect.NewOrgServiceHandler(s.serverSet.OrgHandler, withAuth))
	mux.Handle(registryv1connect.NewCIServiceHandler(s.serverSet.CIHandler, withAuth))
	mux.Handle(registryv1connect.NewNotificationServiceHandler(s.serverSet.NotificationHandler, withAuth))
	mux.Handle(registryv1connect.NewTreeServiceHandler(s.serverSet.TreeHandler, withAuth))

	if s.serverSet.GoProxyHandler != nil {
		mux.Handle("/go/", s.serverSet.GoProxyHandler)
		mux.Handle("/gen/go/", s.serverSet.GoProxyHandler.GoImportHandler())
	}

	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux, nil
}
