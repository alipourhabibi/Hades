package server

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/authorization/v1/authorizationv1connect"
	"github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/config"
	authenticationservice "github.com/alipourhabibi/Hades/pkg/services/authentication"
	authorizationservice "github.com/alipourhabibi/Hades/pkg/services/authorization"
	bufcommitsservice "github.com/alipourhabibi/Hades/pkg/services/bufcommits"
	bufdownloadservice "github.com/alipourhabibi/Hades/pkg/services/bufdownload"
	bufgraphservice "github.com/alipourhabibi/Hades/pkg/services/bufgraph"
	bufmodulesservice "github.com/alipourhabibi/Hades/pkg/services/bufmodules"
	moduleservice "github.com/alipourhabibi/Hades/pkg/services/module"
	"github.com/alipourhabibi/Hades/pkg/services/sdk"
	uploadsservice "github.com/alipourhabibi/Hades/pkg/services/upload"
	"github.com/alipourhabibi/Hades/server/authentication"
	"github.com/alipourhabibi/Hades/server/authorization"
	bufcommits "github.com/alipourhabibi/Hades/server/bufcommits"
	"github.com/alipourhabibi/Hades/server/bufdownload"
	"github.com/alipourhabibi/Hades/server/bufgraph"
	bufmodules "github.com/alipourhabibi/Hades/server/bufmodules"
	"github.com/alipourhabibi/Hades/server/hook"
	"github.com/alipourhabibi/Hades/server/module"
	"github.com/alipourhabibi/Hades/server/upload"
	"github.com/alipourhabibi/Hades/storage/db"
	"github.com/alipourhabibi/Hades/storage/gitaly"
	errorsutils "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/util"
)

// SchemaRegistryServer is the API server for SchemaRegistry
type SchemaRegistryServer struct {
	logger *log.LoggerWrapper

	db     *db.DBs
	gitaly *gitaly.StorageService

	listenPort int
	certFile   string
	keyFile    string

	serverSet *SchemaRegistryServerSet
}

// SchemaRegistryServerSet holds all the server for schema registry
type SchemaRegistryServerSet struct {
	AuthenticationServer *authentication.Server
	AuthorizationServer  *authorization.Server
	ModuleServer         *module.Server
	BufModuleServer      *bufmodules.Server
	BufCommitServer      *bufcommits.Server
	BufUploadServer      *upload.Server
	BufGraphServer       *bufgraph.Server
	BufDownloadServer    *bufdownload.Server
	InternalServer       *hook.Server
}

type SchemaRegistryConfiguration func(*SchemaRegistryServer) error

// WithLogger injects the logger to the SchemaRegistryServer
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

// WithDB injects the database into the SchemaRegistryServer
func WithDB(db *db.DBs) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.db = db
		return nil
	}
}

func WithGitaly(g *gitaly.StorageService) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.gitaly = g
		return nil
	}
}

// NewServer returns a new instance of the Schema Registry API server
func NewServer(ctx context.Context, c *config.Config, cfgs ...SchemaRegistryConfiguration) (*SchemaRegistryServer, error) {

	ss := defaultSchmeaRegistryServer()

	if c.Server.ListenPort != 0 {
		ss.listenPort = c.Server.ListenPort
	}

	ss.certFile = c.Server.CertFile
	ss.keyFile = c.Server.CertKey

	// replace configs with the given configs
	for _, cfg := range cfgs {
		err := cfg(ss)
		if err != nil {
			return nil, err
		}
	}

	return ss, nil
}

// Run runs the SchmeaRegistry server
func (s *SchemaRegistryServer) Run(ctx context.Context, cancel context.CancelFunc) {

	var err error
	// this should be another place?
	s.serverSet, err = newSchemaRegistryServerSet(s)
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}

	mux := s.newServerMux()

	s.logger.Info("StartingServer...", "port", s.listenPort)
	err = http.ListenAndServeTLS(fmt.Sprintf(":%d", 443), s.certFile, s.keyFile, h2c.NewHandler(mux, &http2.Server{}))
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}
}
func newSchemaRegistryServerSet(s *SchemaRegistryServer) (*SchemaRegistryServerSet, error) {

	serverSet := &SchemaRegistryServerSet{}

	casbinAdapter, err := gormadapter.NewAdapterByDB(s.db.CasbinStorage.GetDB())
	if err != nil {
		return nil, err
	}
	casbinEnforcer, err := casbin.NewEnforcer("config/rbac_model.conf", casbinAdapter)
	if err != nil {
		return nil, err
	}
	casbinEnforcer.AddNamedDomainMatchingFunc("g", "keyMatch2", util.KeyMatch2)

	authorizationService, err := authorizationservice.New(
		authorizationservice.WithSessionStorage(s.db.SessionStorage),
		authorizationservice.WithUserStorage(s.db.UserStorage),
		authorizationservice.WithCasbinEnforcer(casbinEnforcer),
	)
	if err != nil {
		return nil, err
	}

	authenticationService, err := authenticationservice.New(s.db.UserStorage, s.db.SessionStorage, authorizationService)
	if err != nil {
		return nil, err
	}

	moduleService, err := moduleservice.New(
		s.db.ModuleStorage,
		s.db.CommitStorage,
		s.gitaly.RepositoryService,
		s.gitaly.OperattionService,
		authorizationService,
	)
	if err != nil {
		return nil, err
	}

	generatorService := &sdk.Generator{}

	bufmoduleService, err := bufmodulesservice.New(s.db.ModuleStorage, s.db.CommitStorage, authorizationService)
	if err != nil {
		return nil, err
	}

	bufcommitService, err := bufcommitsservice.New(s.logger, s.db.CommitStorage, s.db.ModuleStorage, authorizationService)
	if err != nil {
		return nil, err
	}

	uploadService, err := uploadsservice.NewService(s.logger, s.gitaly.CommitService, s.gitaly.OperattionService, s.db.ModuleStorage, s.db.CommitStorage, s.gitaly.BlobService, authorizationService, generatorService)
	if err != nil {
		return nil, err
	}

	graphService, err := bufgraphservice.New(s.logger, s.db.CommitStorage, s.db.ModuleStorage, authorizationService)
	if err != nil {
		return nil, err
	}

	downloadService, err := bufdownloadservice.New(s.logger, s.db.ModuleStorage, s.db.CommitStorage, s.gitaly.BlobService, authorizationService)
	if err != nil {
		return nil, err
	}

	authenticationServer := authentication.NewServer(s.logger, authenticationService)
	authorizationServer := authorization.NewServer(s.logger, authorizationService)
	moduleServer := module.NewServer(s.logger, moduleService)
	bufModuleServer := bufmodules.NewServer(s.logger, bufmoduleService)
	bufCommitServer := bufcommits.NewServer(s.logger, bufcommitService)
	uploadServer := upload.NewServer(s.logger, uploadService)
	bufGraphServer := bufgraph.NewServer(s.logger, graphService)
	bufDownloadServer := bufdownload.NewServer(s.logger, downloadService)
	internalServer := hook.NewServer(s.logger)

	serverSet.AuthenticationServer = authenticationServer
	serverSet.AuthorizationServer = authorizationServer
	serverSet.ModuleServer = moduleServer
	serverSet.BufModuleServer = bufModuleServer
	serverSet.BufUploadServer = uploadServer
	serverSet.BufCommitServer = bufCommitServer
	serverSet.BufGraphServer = bufGraphServer
	serverSet.BufDownloadServer = bufDownloadServer
	serverSet.InternalServer = internalServer

	return serverSet, nil
}

// newServerMux creates the server mux based on the attributes in SchemaRegistryServer
func (s *SchemaRegistryServer) newServerMux() *http.ServeMux {
	mux := http.NewServeMux()

	interceptorsList := []connect.Interceptor{
		s.serverSet.AuthorizationServer.NewAuthorizationInterceptor(),
		errorsutils.NewErrorInterceptor(),
	}

	interceptors := connect.WithInterceptors(
		interceptorsList...,
	)

	noAuthInterceptors := connect.WithInterceptors(
		interceptorsList[1:]...,
	)

	reflector := grpcreflect.NewStaticReflector(
		// Register all your services with the reflector
		authenticationv1connect.AuthenticationServiceName,
		authorizationv1connect.AuthorizationName,
		registryv1connect.ModuleServiceName,
	)

	authenticationPath, authenticationHandler := authenticationv1connect.NewAuthenticationServiceHandler(s.serverSet.AuthenticationServer, noAuthInterceptors)
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

	mux.Handle("/api/v4/internal/", http.Handler(s.serverSet.InternalServer))

	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux
}
