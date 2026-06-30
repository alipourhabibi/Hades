// Package hades wires all service handlers into an HTTP server and
// manages the server lifecycle.
package hades

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/goproxy"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
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
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gitfactory"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/storagefactory"
	emailutils "github.com/alipourhabibi/Hades/utils/email"
	"github.com/alipourhabibi/Hades/utils/log"
)

// SchemaRegistryServer is the top-level server. Constructed by NewServer; started by Run.
type SchemaRegistryServer struct {
	logger     *log.LoggerWrapper
	db         *db.DBs
	gitStorage git.Storage
	config     *config.Config
	serverSet  *SchemaRegistryServerSet
	listenPort int
	certFile   string
	keyFile    string
}

// SchemaRegistryServerSet holds all Connect-RPC service handlers.
type SchemaRegistryServerSet struct {
	AuthenticationServer *authentication.Server
	AuthorizationServer  *authorization.Server
	ModuleServer         *module.Server
	BufModuleServer      *bufmodules.Server
	BufCommitServer      *bufcommits.Server
	BufUploadServer      *bufupload.Server
	BufGraphServer       *bufgraph.Server
	BufDownloadServer    *bufdownload.Server
	SessionHandler       *sessionsvc.Handler
	OAuthHandler         *oauthsvc.Handler
	APITokenHandler      *apitokensvc.Handler
	DeviceHandler        *devicesvc.Handler
	TOTPHandler          *totpsvc.Handler
	AuditHandler         *auditsvc.Handler
	CommitHandler        *commitsvc.Handler
	DiffHandler          *diffsvc.Handler
	UserHandler          *usersvc.Handler
	SDKHandler           *sdksvc.Handler
	OrgHandler           *orgsvc.Handler
	CIHandler            *cisvc.Handler
	NotificationHandler  *notificationsvc.Handler
	TreeHandler          *treesvc.Handler
	GoProxyHandler       *goproxy.Handler
	SDKBackend           sdkstorage.Backend
}

// NewServer constructs a fully wired SchemaRegistryServer from config.
// All backends, service handlers, and routing are initialised here.
// Call Run to start listening.
func NewServer(ctx context.Context, c *config.Config) (*SchemaRegistryServer, error) {
	logger, err := newLogger(c.Logger)
	if err != nil {
		return nil, fmt.Errorf("server: logger: %w", err)
	}

	dbBackend, err := db.NewFromConfig(*c, logger)
	if err != nil {
		return nil, fmt.Errorf("server: db: %w", err)
	}

	gitStorage, err := gitfactory.NewFromConfig(c)
	if err != nil {
		return nil, fmt.Errorf("server: git: %w", err)
	}

	ss := &SchemaRegistryServer{
		listenPort: 50051,
		config:     c,
		certFile:   c.Server.CertFile,
		keyFile:    c.Server.CertKey,
		logger:     logger,
		db:         dbBackend,
		gitStorage: gitStorage,
	}
	if c.Server.ListenPort != 0 {
		ss.listenPort = c.Server.ListenPort
	}

	opaEngine, err := authorizationengine.New(ctx, ss.db.OPABindingStorage)
	if err != nil {
		return nil, fmt.Errorf("server: opa engine: %w", err)
	}

	authorizationServer := authorization.NewServer(ss.logger, ss.db.UserStorage, ss.db.SessionStorage, opaEngine)
	authorizationServer.
		WithAPITokenStorage(ss.db.APITokenStorage).
		WithTOTPSecretStorage(ss.db.TOTPSecretStorage)

	sdkBackend, err := storagefactory.New(*c, ss.gitStorage)
	if err != nil {
		return nil, fmt.Errorf("server: sdk artifact storage: %w", err)
	}

	cacheBackend, err := cache.New(c.Backends, c.Redis)
	if err != nil {
		return nil, fmt.Errorf("server: cache: %w", err)
	}

	deps := &server.Dependencies{
		Logger:              ss.logger,
		OPAEngine:           opaEngine,
		Authorization:       authorizationServer,
		ModuleDB:            ss.db.ModuleStorage,
		CommitDB:            ss.db.CommitStorage,
		SDKJobDB:            ss.db.SDKJobStorage,
		SDKStorageBackend:   sdkBackend,
		OrgDB:               ss.db.OrgStorage,
		CIRunDB:             ss.db.CIRunStorage,
		NotificationDB:      ss.db.NotificationStorage,
		GitStorage:          ss.gitStorage,
		GitalyOpLog:         ss.db.GitalyOpLogStorage,
		UserDB:              ss.db.UserStorage,
		SessionDB:           ss.db.SessionStorage,
		UoW:                 ss.db.UOW,
		SDKConfig:           c.SDK,
		ProtoLinter:         lint.New(c.SDK.BufBin),
		BreakingChk:         breaking.New(c.SDK.BufBin),
		EmailVerificationDB: ss.db.EmailVerificationStorage,
		PasswordResetDB:     ss.db.PasswordResetStorage,
		OAuthIdentityDB:     ss.db.OAuthIdentityStorage,
		APITokenDB:          ss.db.APITokenStorage,
		DeviceGrantDB:       ss.db.DeviceGrantStorage,
		TOTPSecretDB:        ss.db.TOTPSecretStorage,
		BackupCodeDB:        ss.db.BackupCodeStorage,
		AuditLogDB:          ss.db.AuditLogStorage,
		Cache:               cacheBackend,
		EmailSender:         emailutils.New(c.Email, ss.logger),
		AuthConfig:          c.Auth,
		TOTPConfig:          c.TOTP,
		OAuthConfig:         c.OAuth,
		RegistryHost:        c.Server.RegistryHost,
	}

	ss.serverSet = &SchemaRegistryServerSet{
		AuthorizationServer: authorizationServer,
		AuthenticationServer: authentication.NewServer(deps),
		ModuleServer:        module.NewServer(deps),
		BufModuleServer:     bufmodules.NewServer(deps),
		BufCommitServer:     bufcommits.NewServer(deps),
		BufUploadServer:     bufupload.NewServer(deps),
		BufGraphServer:      bufgraph.NewServer(deps),
		BufDownloadServer:   bufdownload.NewServer(deps),
		SessionHandler:      sessionsvc.NewHandler(deps),
		OAuthHandler:        oauthsvc.NewHandler(deps),
		APITokenHandler:     apitokensvc.NewHandler(deps),
		DeviceHandler:       devicesvc.NewHandler(deps),
		TOTPHandler:         totpsvc.NewHandler(deps),
		AuditHandler:        auditsvc.NewHandler(deps),
		CommitHandler:       commitsvc.NewHandler(deps),
		DiffHandler:         diffsvc.NewHandler(deps),
		UserHandler:         usersvc.NewHandler(deps),
		SDKHandler:          sdksvc.NewHandler(deps),
		OrgHandler:          orgsvc.NewHandler(deps),
		CIHandler:           cisvc.NewHandler(deps),
		NotificationHandler: notificationsvc.NewHandler(deps),
		TreeHandler:         treesvc.NewHandler(deps),
		GoProxyHandler:      goproxy.NewHandler(deps, c.Server.RegistryHost),
		SDKBackend:          sdkBackend,
	}

	return ss, nil
}

// newLogger constructs a logger from config.
func newLogger(c config.Logger) (*log.LoggerWrapper, error) {
	switch c.Engine {
	case log.Zap:
		return log.NewZapWithConfig(c)
	default:
		return log.NewWithConfig(c)
	}
}
