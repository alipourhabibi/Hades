package hades

import (
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/otel"

	registryv1alpha1connect "buf.build/gen/go/bufbuild/buf/connectrpc/go/buf/alpha/registry/v1alpha1/registryv1alpha1connect"
	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/auth/v1/authv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	identityv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
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
		middleware.NewRequestLoggingInterceptor(s.logger),
		protovalidateInterceptor,
		otelInterceptor,
		errorsutils.NewErrorInterceptor(),
	}

	withAuth := connect.WithInterceptors(append(
		[]connect.Interceptor{s.serverSet.AuthorizationServer.NewAuthorizationInterceptor()},
		base...,
	)...)

	reflector := grpcreflect.NewStaticReflector(
		authv1connect.AuthenticationServiceName,
		authv1connect.SessionServiceName,
		authv1connect.OAuthServiceName,
		authv1connect.APITokenServiceName,
		authv1connect.DeviceServiceName,
		authv1connect.TOTPServiceName,
		authv1connect.AuditServiceName,
		authorizationv1connect.AuthorizationName,
		registryv1connect.ModuleServiceName,
		registryv1connect.CommitServiceName,
		registryv1connect.SDKServiceName,
		registryv1connect.CIServiceName,
		identityv1connect.UserServiceName,
		identityv1connect.OrgServiceName,
		identityv1connect.NotificationServiceName,
		registryv1alpha1connect.AuthnServiceName,
	)

	mux := http.NewServeMux()

	// Auth-domain handlers - all served by the single *auth.Server
	mux.Handle(authv1connect.NewAuthenticationServiceHandler(s.serverSet.AuthServer, withAuth))
	mux.Handle(authv1connect.NewSessionServiceHandler(s.serverSet.AuthServer, withAuth))
	// OAuth and Device services carry both pre-session procedures (GetOAuthURL,
	// OAuthCallback, RequestDeviceCode, PollDeviceToken) and procedures that
	// need an authenticated user (ApproveDeviceGrant, UnlinkProvider,
	// ListLinkedProviders). They are mounted with the auth interceptor; the
	// pre-session procedures are exempted by noAuthProcedures in the
	// interceptor itself. Mounting them without it left the authenticated
	// procedures permanently unable to see a user.
	mux.Handle(authv1connect.NewOAuthServiceHandler(s.serverSet.AuthServer, withAuth))
	mux.Handle(authv1connect.NewAPITokenServiceHandler(s.serverSet.AuthServer, withAuth))
	mux.Handle(authv1connect.NewDeviceServiceHandler(s.serverSet.AuthServer, withAuth))
	mux.Handle(authv1connect.NewTOTPServiceHandler(s.serverSet.AuthServer, withAuth))
	mux.Handle(authv1connect.NewAuditServiceHandler(s.serverSet.AuthServer, withAuth))

	mux.Handle(authorizationv1connect.NewAuthorizationHandler(s.serverSet.AuthorizationServer, withAuth))
	mux.Handle(registryv1connect.NewModuleServiceHandler(s.serverSet.ModuleServer, withAuth))
	mux.Handle(registryv1connect.NewCommitServiceHandler(s.serverSet.CommitHandler, withAuth))
	mux.Handle(registryv1connect.NewCIServiceHandler(s.serverSet.MetaHandler, withAuth))
	mux.Handle(registryv1connect.NewSDKServiceHandler(s.serverSet.MetaHandler, withAuth))
	mux.Handle(identityv1connect.NewUserServiceHandler(s.serverSet.IdentityHandler, withAuth))
	mux.Handle(identityv1connect.NewOrgServiceHandler(s.serverSet.IdentityHandler, withAuth))
	mux.Handle(identityv1connect.NewNotificationServiceHandler(s.serverSet.NotifHandler, withAuth))

	// buf.build protocol adapters
	mux.Handle(modulev1connect.NewModuleServiceHandler(s.serverSet.BufModuleServer, withAuth))
	mux.Handle(modulev1connect.NewCommitServiceHandler(s.serverSet.BufCommitServer, withAuth))
	mux.Handle(modulev1connect.NewUploadServiceHandler(s.serverSet.BufUploadServer, withAuth))
	mux.Handle(modulev1connect.NewGraphServiceHandler(s.serverSet.BufGraphServer, withAuth))
	mux.Handle(modulev1connect.NewDownloadServiceHandler(s.serverSet.BufDownloadServer, withAuth))
	mux.Handle(registryv1alpha1connect.NewAuthnServiceHandler(s.serverSet.BufAlphaAuthnServer, withAuth))

	if s.serverSet.GoProxyHandler != nil {
		mux.Handle("/go/", s.serverSet.GoProxyHandler)
		mux.Handle("/gen/go/", s.serverSet.GoProxyHandler.GoImportHandler())
	}

	// gRPC reflection publishes the full service and method schema to anyone who
	// can reach the port, so it is opt-in via server.enableReflection.
	if s.config != nil && s.config.Server.EnableReflection {
		mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}

	return mux, nil
}
