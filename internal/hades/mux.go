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
	"github.com/alipourhabibi/Hades/utils/connerr"
)

// newServerMux registers all Connect-RPC and gRPC reflection handlers.
func (s *SchemaRegistryServer) newServerMux() (*http.ServeMux, error) {
	protovalidateInterceptor, err := middleware.NewProtovalidateInterceptor()
	if err != nil {
		return nil, fmt.Errorf("protovalidate interceptor: %w", err)
	}

	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithTracerProvider(otel.GetTracerProvider()),
		otelconnect.WithMeterProvider(otel.GetMeterProvider()),
		// net.peer.port is the client's ephemeral source port, so it is a
		// different value for essentially every TCP connection. otelconnect
		// puts it in the attribute set of five histogram instruments, and the
		// metric SDK retains one time series per unique attribute set for the
		// lifetime of the process. Left on, server memory grows with the total
		// number of connections ever accepted and is never released: a 20
		// second load stage took RSS from 61 MB to 6.55 GB during the E2E run.
		// net.peer.name goes with it, which is fine here: the useful dimensions
		// are the procedure and the status code, both of which are kept.
		otelconnect.WithoutServerPeerAttributes(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel interceptor: %w", err)
	}

	// Interceptor order. The first element is the outermost.
	//
	//  1. otel            - outside authentication, so requests rejected by authn
	//                       or authz still produce a span and an RPC metric. A
	//                       credential-stuffing run is meant to be visible in the
	//                       dashboards that exist to show it.
	//  2. error           - a translation layer belongs at the boundary, so that
	//                       errors raised by authorization, protovalidate and the
	//                       request logger also pass through ToConnectError.
	//  3. authorization   - authenticate and authorize before anything reads the
	//                       message.
	//  4. protovalidate   - reject malformed messages...
	//  5. request logging - ...before the logger serialises them.
	chain := []connect.Interceptor{
		otelInterceptor,
		connerr.NewErrorInterceptor(),
		s.serverSet.AuthorizationServer.NewAuthorizationInterceptor(),
		protovalidateInterceptor,
		middleware.NewRequestLoggingInterceptor(s.logger),
	}

	withAuth := connect.WithInterceptors(chain...)

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
		// The buf protocol is the compatibility surface people integrate
		// against, and these five were mounted below without being advertised
		// here: a reflection-driven client or a grpcurl exploration could not
		// see the services that matter most. The v1alpha AuthnService was in
		// the list, so the omission was drift as the buf services were added
		// rather than a decision.
		modulev1connect.ModuleServiceName,
		modulev1connect.CommitServiceName,
		modulev1connect.UploadServiceName,
		modulev1connect.GraphServiceName,
		modulev1connect.DownloadServiceName,
	)

	mux := http.NewServeMux()

	// Liveness endpoint.
	//
	// It exists so the container HEALTHCHECK and any orchestrator probe have
	// something cheap and unauthenticated to ask. It reports that the process
	// is up and routing, and deliberately nothing else: a readiness check that
	// touched the database would make a database blip restart the server.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

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
		// Both handlers. grpc.reflection.v1 is the stable API and v1alpha is
		// the deprecated one; only v1alpha was registered, so the v1 path was
		// a 404 and a client that speaks only v1 got Unimplemented. grpcurl
		// falls back to v1alpha, which is what hid this.
		mux.Handle(grpcreflect.NewHandlerV1(reflector))
		mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))
	}

	return mux, nil
}
