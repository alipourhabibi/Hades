// Package hades wires all service handlers into an HTTP server and
// manages the server lifecycle.
package hades

import (
	"context"
	"fmt"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/goproxy"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	authsvc "github.com/alipourhabibi/Hades/internal/hades/server/auth"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/server/buf/authn"
	bufcommits "github.com/alipourhabibi/Hades/internal/hades/server/buf/commits"
	"github.com/alipourhabibi/Hades/internal/hades/server/buf/download"
	"github.com/alipourhabibi/Hades/internal/hades/server/buf/graph"
	bufmodules "github.com/alipourhabibi/Hades/internal/hades/server/buf/modules"
	"github.com/alipourhabibi/Hades/internal/hades/server/buf/upload"
	commitsvc "github.com/alipourhabibi/Hades/internal/hades/server/commit"
	contentsvc "github.com/alipourhabibi/Hades/internal/hades/server/content"
	identitysvc "github.com/alipourhabibi/Hades/internal/hades/server/identity"
	metasvc "github.com/alipourhabibi/Hades/internal/hades/server/meta"
	"github.com/alipourhabibi/Hades/internal/hades/server/module"
	notificationsvc "github.com/alipourhabibi/Hades/internal/hades/server/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gitfactory"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/storagefactory"
	"github.com/alipourhabibi/Hades/utils/clientip"
	emailutils "github.com/alipourhabibi/Hades/utils/email"
	"github.com/alipourhabibi/Hades/utils/log"
)

// SchemaRegistryServer is the top-level server. Constructed by NewServer; started by Run.
type SchemaRegistryServer struct {
	logger     *log.LoggerWrapper
	db         db.Store
	gitStorage git.Storage
	config     *config.Config
	serverSet  *SchemaRegistryServerSet
	listenPort int
	certFile   string
	keyFile    string
}

// SchemaRegistryServerSet holds all Connect-RPC service handlers.
type SchemaRegistryServerSet struct {
	AuthServer          *authsvc.Server
	AuthorizationServer *authorization.Server
	ModuleServer        *module.Server
	CommitHandler       *commitsvc.Handler
	ContentHandler      *contentsvc.Handler
	MetaHandler         *metasvc.Handler
	IdentityHandler     *identitysvc.Handler
	NotifHandler        *notificationsvc.Handler
	BufModuleServer     *bufmodules.Server
	BufCommitServer     *bufcommits.Server
	BufUploadServer     *bufupload.Server
	BufGraphServer      *bufgraph.Server
	BufDownloadServer   *bufdownload.Server
	BufAlphaAuthnServer *bufauthn.Server
	GoProxyHandler      *goproxy.Handler
	SDKBackend          sdkstorage.Backend
}

// NewServer constructs a fully wired SchemaRegistryServer from config.
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

	cacheBackend, err := cache.New(c.Backends, c.Redis)
	if err != nil {
		return nil, fmt.Errorf("server: cache: %w", err)
	}

	opaTTL := c.OPA.BindingCacheTTL
	if opaTTL == 0 {
		if c.Backends.Cache == config.CacheRedis {
			opaTTL = 60 * time.Second
		} else {
			opaTTL = 10 * time.Second
		}
	}
	opaEngine, err := authorizationengine.New(ctx, ss.db.OPABinding(), cacheBackend, opaTTL)
	if err != nil {
		return nil, fmt.Errorf("server: opa engine: %w", err)
	}

	authorizationServer := authorization.NewServer(ss.logger, ss.db.User(), ss.db.Session(), opaEngine)
	authorizationServer.
		WithAPITokenStorage(ss.db.APIToken()).
		WithTOTPSecretStorage(ss.db.TOTPSecret())

	sdkBackend, err := storagefactory.New(*c, ss.gitStorage)
	if err != nil {
		return nil, fmt.Errorf("server: sdk artifact storage: %w", err)
	}

	trustedProxies, err := clientip.ParseTrustedProxies(c.Server.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("server: server.trustedProxies: %w", err)
	}

	deps := &server.Dependencies{
		TrustedProxies:      trustedProxies,
		Logger:              ss.logger,
		OPAEngine:           opaEngine,
		Authorization:       authorizationServer,
		ModuleDB:            ss.db.Module(),
		CommitDB:            ss.db.Commit(),
		ResourceDB:          ss.db.Resource(),
		SDKJobDB:            ss.db.SDKJob(),
		SDKStorageBackend:   sdkBackend,
		OrgDB:               ss.db.Org(),
		CIRunDB:             ss.db.CIRun(),
		NotificationDB:      ss.db.Notification(),
		GitStorage:          ss.gitStorage,
		GitalyOpLog:         ss.db.GitalyOpLog(),
		UserDB:              ss.db.User(),
		SessionDB:           ss.db.Session(),
		UoW:                 ss.db,
		SDKConfig:           c.SDK,
		ProtoLinter:         lint.New(c.SDK.BufBin),
		BreakingChk:         breaking.New(c.SDK.BufBin),
		EmailVerificationDB: ss.db.EmailVerification(),
		PasswordResetDB:     ss.db.PasswordReset(),
		OAuthIdentityDB:     ss.db.OAuthIdentity(),
		APITokenDB:          ss.db.APIToken(),
		DeviceGrantDB:       ss.db.DeviceGrant(),
		TOTPSecretDB:        ss.db.TOTPSecret(),
		BackupCodeDB:        ss.db.BackupCode(),
		AuditLogDB:          ss.db.AuditLog(),
		Cache:               cacheBackend,
		EmailSender:         emailutils.New(c.Email, ss.logger),
		AuthConfig:          c.Auth,
		TOTPConfig:          c.TOTP,
		OAuthConfig:         c.OAuth,
		RegistryHost:        c.Server.RegistryHost,
	}

	ss.serverSet = &SchemaRegistryServerSet{
		AuthorizationServer: authorizationServer,
		AuthServer:          authsvc.NewServer(deps),
		ModuleServer:        module.NewServer(deps),
		CommitHandler:       commitsvc.NewHandler(deps),
		ContentHandler:      contentsvc.NewHandler(deps),
		MetaHandler:         metasvc.NewHandler(deps),
		IdentityHandler:     identitysvc.NewHandler(deps),
		NotifHandler:        notificationsvc.NewHandler(deps),
		BufModuleServer:     bufmodules.NewServer(deps),
		BufCommitServer:     bufcommits.NewServer(deps),
		BufUploadServer:     bufupload.NewServer(deps),
		BufGraphServer:      bufgraph.NewServer(deps),
		BufDownloadServer:   bufdownload.NewServer(deps),
		BufAlphaAuthnServer: bufauthn.NewServer(deps),
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
