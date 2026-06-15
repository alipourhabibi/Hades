// Package hades wires all service handlers into an HTTP server and
// manages the server lifecycle. It is the main integration point for
// the schema registry.
package hades

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/otel"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/goproxy"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/apitokensvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/auditsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/authentication"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	bufcommits "github.com/alipourhabibi/Hades/internal/hades/server/bufcommits"
	"github.com/alipourhabibi/Hades/internal/hades/server/bufdownload"
	"github.com/alipourhabibi/Hades/internal/hades/server/bufgraph"
	bufmodules "github.com/alipourhabibi/Hades/internal/hades/server/bufmodules"
	"github.com/alipourhabibi/Hades/internal/hades/server/bufupload"
	"github.com/alipourhabibi/Hades/internal/hades/server/cisvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/commitsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/devicesvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/diffsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/middleware"
	"github.com/alipourhabibi/Hades/internal/hades/server/module"
	"github.com/alipourhabibi/Hades/internal/hades/server/notificationsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/oauthsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/orgsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/sdksvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/sessionsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/totpsvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/treesvc"
	"github.com/alipourhabibi/Hades/internal/hades/server/usersvc"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	"github.com/alipourhabibi/Hades/internal/sdk/generate"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/storage/s3"
	"github.com/alipourhabibi/Hades/internal/sdk/worker"
	emailutils "github.com/alipourhabibi/Hades/utils/email"
	errorsutils "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/ratelimit"
	"github.com/redis/go-redis/v9"
)

// SchemaRegistryServer is the top-level API server. It owns the listener
// configuration and delegates request handling to the handlers in
// SchemaRegistryServerSet.
type SchemaRegistryServer struct {
	logger *log.LoggerWrapper

	db     *db.DBs
	gitaly *gitaly.StorageService
	config *config.Config

	listenPort int
	certFile   string
	keyFile    string

	serverSet *SchemaRegistryServerSet
}

// SchemaRegistryServerSet groups all Connect-RPC service handlers that
// are registered on the HTTP mux.
type SchemaRegistryServerSet struct {
	AuthenticationServer *authentication.Server
	AuthorizationServer  *authorization.Server
	ModuleServer         *module.Server
	BufModuleServer      *bufmodules.Server
	BufCommitServer      *bufcommits.Server
	BufUploadServer      *bufupload.Server
	BufGraphServer       *bufgraph.Server
	BufDownloadServer    *bufdownload.Server

	// Authentication service handlers.
	SessionHandler  *sessionsvc.Handler
	OAuthHandler    *oauthsvc.Handler
	APITokenHandler *apitokensvc.Handler
	DeviceHandler   *devicesvc.Handler
	TOTPHandler     *totpsvc.Handler
	AuditHandler    *auditsvc.Handler

	// Registry service handlers.
	CommitHandler       *commitsvc.Handler
	DiffHandler         *diffsvc.Handler
	UserHandler         *usersvc.Handler
	SDKHandler          *sdksvc.Handler
	OrgHandler          *orgsvc.Handler
	CIHandler           *cisvc.Handler
	NotificationHandler *notificationsvc.Handler
	TreeHandler         *treesvc.Handler
	GoProxyHandler      *goproxy.Handler

	// SDKBackend is shared between the SDK worker and Go proxy handler.
	SDKBackend sdkstorage.Backend
}

// SchemaRegistryConfiguration is a functional option for SchemaRegistryServer.
type SchemaRegistryConfiguration func(*SchemaRegistryServer) error

// WithLogger sets the structured logger on the server.
func WithLogger(logger *log.LoggerWrapper) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.logger = logger
		return nil
	}
}

func defaultSchmeaRegistryServer() *SchemaRegistryServer {
	return &SchemaRegistryServer{
		listenPort: 50051,
		logger:     log.DefaultLogger(),
	}
}

// WithDB sets the database storage layer on the server.
func WithDB(db *db.DBs) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.db = db
		return nil
	}
}

// WithGitaly sets the Gitaly gRPC storage backend on the server.
func WithGitaly(g *gitaly.StorageService) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.gitaly = g
		return nil
	}
}

// NewServer constructs a SchemaRegistryServer and applies the given
// functional options. The server is not started until Run is called.
func NewServer(ctx context.Context, c *config.Config, cfgs ...SchemaRegistryConfiguration) (*SchemaRegistryServer, error) {
	ss := defaultSchmeaRegistryServer()

	if c.Server.ListenPort != 0 {
		ss.listenPort = c.Server.ListenPort
	}

	ss.certFile = c.Server.CertFile
	ss.keyFile = c.Server.CertKey
	ss.config = c

	for _, cfg := range cfgs {
		if err := cfg(ss); err != nil {
			return nil, err
		}
	}

	return ss, nil
}

// Run initialises all service handlers and starts the HTTP listener.
// It cancels the context on fatal errors.
func (s *SchemaRegistryServer) Run(ctx context.Context, cancel context.CancelFunc) {
	var err error
	s.serverSet, err = newSchemaRegistryServerSet(ctx, s)
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}

	if s.config != nil && s.config.SDK.Enabled {
		sdkWorker, workerErr := newSDKWorker(s, s.serverSet.SDKBackend)
		if workerErr != nil {
			s.logger.Error("Failed to create SDK worker", "error", workerErr)
		} else {
			go sdkWorker.Run(ctx)
		}
	}

	mux, err := s.newServerMux()
	if err != nil {
		s.logger.Error("Failed to create server mux", "error", err)
		cancel()
		return
	}

	handler := h2c.NewHandler(mux, &http2.Server{})
	if s.certFile == "" {
		// Plain HTTP/2 cleartext - expects a TLS-terminating reverse proxy in front.
		s.logger.Info("starting h2c server (no TLS)", "port", s.listenPort)
		err = http.ListenAndServe(fmt.Sprintf(":%d", s.listenPort), handler)
	} else {
		s.logger.Info("starting TLS server", "port", 443)
		err = http.ListenAndServeTLS(":443", s.certFile, s.keyFile, handler)
	}
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}
}

// newSchemaRegistryServerSet constructs all service handlers and injects
// shared dependencies. This is the main dependency wiring function.
func newSchemaRegistryServerSet(ctx context.Context, s *SchemaRegistryServer) (*SchemaRegistryServerSet, error) {
	serverSet := &SchemaRegistryServerSet{}

	opaEngine, err := authorizationengine.New(ctx, s.db.OPABindingStorage)
	if err != nil {
		return nil, fmt.Errorf("opa engine: %w", err)
	}

	authorizationServer := authorization.NewServer(s.logger, s.db.UserStorage, s.db.SessionStorage, opaEngine)
	authorizationServer.
		WithAPITokenStorage(s.db.APITokenStorage).
		WithTOTPSecretStorage(s.db.TOTPSecretStorage)
	serverSet.AuthorizationServer = authorizationServer

	sdkCfg := config.SDKConfig{}
	if s.config != nil {
		sdkCfg = s.config.SDK
	}

	// SDK storage backend is shared between the SDK worker and Go proxy handler.
	var sdkBackend sdkstorage.Backend
	if sdkCfg.Storage.Type == "s3" {
		b, err := s3.New(sdkCfg.Storage.S3)
		if err != nil {
			return nil, fmt.Errorf("sdk s3 backend: %w", err)
		}
		sdkBackend = b
	}

	// Redis-backed rate limiter is optional; nil when Redis is not configured.
	var redisClient *redis.Client
	var rateLimiter *ratelimit.Limiter
	if s.config != nil && s.config.Redis.Addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     s.config.Redis.Addr,
			Password: s.config.Redis.Password,
			DB:       s.config.Redis.DB,
		})
		rateLimiter = ratelimit.New(redisClient)
	}

	// Email sender; uses a stub implementation when SMTP is not configured.
	var emailCfg config.EmailConfig
	if s.config != nil {
		emailCfg = s.config.Email
	}
	emailSender := emailutils.New(emailCfg, s.logger)

	var authCfg config.AuthConfig
	var totpCfg config.TOTPConfig
	var oauthCfg config.OAuthConfig
	if s.config != nil {
		authCfg = s.config.Auth
		totpCfg = s.config.TOTP
		oauthCfg = s.config.OAuth
	}

	dependencies := &server.Dependencies{
		Logger:                  s.logger,
		OPAEngine:               opaEngine,
		Authorization:           serverSet.AuthorizationServer,
		ModuleDB:                s.db.ModuleStorage,
		CommitDB:                s.db.CommitStorage,
		SDKJobDB:                s.db.SDKJobStorage,
		SDKStorageBackend:       sdkBackend,
		OrgDB:                   s.db.OrgStorage,
		CIRunDB:                 s.db.CIRunStorage,
		NotificationDB:          s.db.NotificationStorage,
		GitalyBlobStorage:       s.gitaly.BlobService,
		GitalyRepositoryStorage: s.gitaly.RepositoryService,
		GitalyOperationStorage:  s.gitaly.OperattionService,
		GitalyDiffStorage:       s.gitaly.DiffService,
		GitalyTreeStorage:       s.gitaly.TreeService,
		GitalyOpLog:             s.db.GitalyOpLogStorage,
		UserDB:                  s.db.UserStorage,
		SessionDB:               s.db.SessionStorage,
		UoW:                     s.db.UOW,
		SDKConfig:               sdkCfg,
		ProtoLinter:             lint.New(sdkCfg.BufBin),
		BreakingChk:             breaking.New(sdkCfg.BufBin),

		EmailVerificationDB: s.db.EmailVerificationStorage,
		PasswordResetDB:     s.db.PasswordResetStorage,
		OAuthIdentityDB:     s.db.OAuthIdentityStorage,
		APITokenDB:          s.db.APITokenStorage,
		DeviceGrantDB:       s.db.DeviceGrantStorage,
		TOTPSecretDB:        s.db.TOTPSecretStorage,
		BackupCodeDB:        s.db.BackupCodeStorage,
		AuditLogDB:          s.db.AuditLogStorage,

		Redis:       redisClient,
		RateLimiter: rateLimiter,
		EmailSender: emailSender,
		AuthConfig:  authCfg,
		TOTPConfig:  totpCfg,
		OAuthConfig: oauthCfg,
	}

	authenticationServer := authentication.NewServer(dependencies)
	moduleServer := module.NewServer(dependencies)
	bufModuleServer := bufmodules.NewServer(dependencies)
	bufCommitServer := bufcommits.NewServer(dependencies)
	uploadServer := bufupload.NewServer(dependencies)
	bufGraphServer := bufgraph.NewServer(dependencies)
	bufDownloadServer := bufdownload.NewServer(dependencies)
	sessionHandler := sessionsvc.NewHandler(dependencies)
	oauthHandler := oauthsvc.NewHandler(dependencies)
	apiTokenHandler := apitokensvc.NewHandler(dependencies)
	deviceHandler := devicesvc.NewHandler(dependencies)
	totpHandler := totpsvc.NewHandler(dependencies)
	auditHandler := auditsvc.NewHandler(dependencies)
	commitHandler := commitsvc.NewHandler(dependencies)
	diffHandler := diffsvc.NewHandler(dependencies)
	userHandler := usersvc.NewHandler(dependencies)
	sdkHandler := sdksvc.NewHandler(dependencies)
	orgHandler := orgsvc.NewHandler(dependencies)
	ciHandler := cisvc.NewHandler(dependencies)
	notificationHandler := notificationsvc.NewHandler(dependencies)
	treeHandler := treesvc.NewHandler(dependencies)

	registryHost := ""
	if s.config != nil {
		registryHost = s.config.Server.RegistryHost
	}
	goProxyHandler := goproxy.NewHandler(dependencies, registryHost)

	serverSet.AuthenticationServer = authenticationServer
	serverSet.ModuleServer = moduleServer
	serverSet.BufModuleServer = bufModuleServer
	serverSet.BufUploadServer = uploadServer
	serverSet.BufCommitServer = bufCommitServer
	serverSet.BufGraphServer = bufGraphServer
	serverSet.BufDownloadServer = bufDownloadServer
	serverSet.SessionHandler = sessionHandler
	serverSet.OAuthHandler = oauthHandler
	serverSet.APITokenHandler = apiTokenHandler
	serverSet.DeviceHandler = deviceHandler
	serverSet.TOTPHandler = totpHandler
	serverSet.AuditHandler = auditHandler
	serverSet.CommitHandler = commitHandler
	serverSet.DiffHandler = diffHandler
	serverSet.UserHandler = userHandler
	serverSet.SDKHandler = sdkHandler
	serverSet.OrgHandler = orgHandler
	serverSet.CIHandler = ciHandler
	serverSet.NotificationHandler = notificationHandler
	serverSet.TreeHandler = treeHandler
	serverSet.GoProxyHandler = goProxyHandler
	serverSet.SDKBackend = sdkBackend

	return serverSet, nil
}

// newServerMux registers all Connect-RPC handlers and the gRPC reflection
// service on a single HTTP mux.
func (s *SchemaRegistryServer) newServerMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()

	protovalidateInterceptor, err := middleware.NewProtovalidateInterceptor()
	if err != nil {
		return nil, fmt.Errorf("creating protovalidate interceptor: %w", err)
	}

	otelInterceptor, _ := otelconnect.NewInterceptor(
		otelconnect.WithTracerProvider(otel.GetTracerProvider()),
		otelconnect.WithMeterProvider(otel.GetMeterProvider()),
	)

	baseInterceptors := []connect.Interceptor{
		protovalidateInterceptor,
		otelInterceptor,
		errorsutils.NewErrorInterceptor(),
	}

	interceptors := connect.WithInterceptors(append(
		[]connect.Interceptor{s.serverSet.AuthorizationServer.NewAuthorizationInterceptor()},
		baseInterceptors...,
	)...)

	noAuthInterceptors := connect.WithInterceptors(baseInterceptors...)

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

	// The authorization interceptor skips Login/Register via noAuthProcedures.
	authenticationPath, authenticationHandler := authenticationv1connect.NewAuthenticationServiceHandler(s.serverSet.AuthenticationServer, interceptors)
	mux.Handle(authenticationPath, authenticationHandler)

	authorizationPath, authorizationHandler := authorizationv1connect.NewAuthorizationHandler(s.serverSet.AuthorizationServer, interceptors)
	mux.Handle(authorizationPath, authorizationHandler)

	modulePath, moduleHandler := registryv1connect.NewModuleServiceHandler(s.serverSet.ModuleServer, interceptors)
	mux.Handle(modulePath, moduleHandler)

	bufmodulePath, bufmoduleHandler := modulev1connect.NewModuleServiceHandler(s.serverSet.BufModuleServer, interceptors)
	mux.Handle(bufmodulePath, bufmoduleHandler)

	bufcommitPath, bufcommitHandler := modulev1connect.NewCommitServiceHandler(s.serverSet.BufCommitServer, interceptors)
	mux.Handle(bufcommitPath, bufcommitHandler)

	bufuploadPath, bufuploadHandler := modulev1connect.NewUploadServiceHandler(s.serverSet.BufUploadServer, interceptors)
	mux.Handle(bufuploadPath, bufuploadHandler)

	bufgraphPath, bufgraphHandler := modulev1connect.NewGraphServiceHandler(s.serverSet.BufGraphServer, interceptors)
	mux.Handle(bufgraphPath, bufgraphHandler)

	bufdownloadPath, bufdownloadHandler := modulev1connect.NewDownloadServiceHandler(s.serverSet.BufDownloadServer, interceptors)
	mux.Handle(bufdownloadPath, bufdownloadHandler)

	sessionPath, sessionHandler := authenticationv1connect.NewSessionServiceHandler(s.serverSet.SessionHandler, interceptors)
	mux.Handle(sessionPath, sessionHandler)

	oauthPath, oauthHandler := authenticationv1connect.NewOAuthServiceHandler(s.serverSet.OAuthHandler, noAuthInterceptors)
	mux.Handle(oauthPath, oauthHandler)

	apiTokenPath, apiTokenHandler := authenticationv1connect.NewAPITokenServiceHandler(s.serverSet.APITokenHandler, interceptors)
	mux.Handle(apiTokenPath, apiTokenHandler)

	devicePath, deviceHandler := authenticationv1connect.NewDeviceServiceHandler(s.serverSet.DeviceHandler, noAuthInterceptors)
	mux.Handle(devicePath, deviceHandler)

	totpPath, totpHandler := authenticationv1connect.NewTOTPServiceHandler(s.serverSet.TOTPHandler, interceptors)
	mux.Handle(totpPath, totpHandler)

	auditPath, auditHandler := authenticationv1connect.NewAuditServiceHandler(s.serverSet.AuditHandler, interceptors)
	mux.Handle(auditPath, auditHandler)

	commitSvcPath, commitSvcHandler := registryv1connect.NewCommitServiceHandler(s.serverSet.CommitHandler, interceptors)
	mux.Handle(commitSvcPath, commitSvcHandler)

	diffSvcPath, diffSvcHandler := registryv1connect.NewDiffServiceHandler(s.serverSet.DiffHandler, interceptors)
	mux.Handle(diffSvcPath, diffSvcHandler)

	userSvcPath, userSvcHandler := registryv1connect.NewUserServiceHandler(s.serverSet.UserHandler, interceptors)
	mux.Handle(userSvcPath, userSvcHandler)

	sdkPath, sdkHandler := registryv1connect.NewSDKServiceHandler(s.serverSet.SDKHandler, interceptors)
	mux.Handle(sdkPath, sdkHandler)

	orgPath, orgHandler := registryv1connect.NewOrgServiceHandler(s.serverSet.OrgHandler, interceptors)
	mux.Handle(orgPath, orgHandler)

	ciPath, ciSvcHandler := registryv1connect.NewCIServiceHandler(s.serverSet.CIHandler, interceptors)
	mux.Handle(ciPath, ciSvcHandler)

	notificationPath, notificationHandler := registryv1connect.NewNotificationServiceHandler(s.serverSet.NotificationHandler, interceptors)
	mux.Handle(notificationPath, notificationHandler)

	treeSvcPath, treeSvcHandler := registryv1connect.NewTreeServiceHandler(s.serverSet.TreeHandler, interceptors)
	mux.Handle(treeSvcPath, treeSvcHandler)

	// GOPROXY protocol handler at /go/{module}/@v/...
	if s.serverSet.GoProxyHandler != nil {
		mux.Handle("/go/", s.serverSet.GoProxyHandler)
		// go-import meta tag at /gen/go/ for "go get" discovery (matches
		// module paths of the form {host}/gen/go/{owner}/{module}).
		mux.Handle("/gen/go/", s.serverSet.GoProxyHandler.GoImportHandler())
	}

	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux, nil
}

// newSDKWorker builds a worker.Worker, reusing the already-initialised
// storage backend so it is shared with the Go proxy handler.
func newSDKWorker(s *SchemaRegistryServer, backend sdkstorage.Backend) (*worker.Worker, error) {
	cfg := s.config.SDK

	generators := make(map[string]*generate.Generator, len(cfg.Generators))
	for _, g := range cfg.Generators {
		generators[g.Plugin] = generate.New(cfg.ProtocBin, g)
	}

	return worker.New(
		s.db.SDKJobStorage,
		s.db.CommitStorage,
		s.gitaly.BlobService,
		generators,
		backend,
		s.logger,
		10*time.Second,
		4,
	), nil
}
