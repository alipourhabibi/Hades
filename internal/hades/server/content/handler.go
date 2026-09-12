// Package content consolidates upload and download logic.
package content

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	notificationdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	orgdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler handles both upload and download operations.
type Handler struct {
	moduleDB        moduledb.Storage
	commitDB        commitdb.Storage
	gitStorage      gitstorage.Storage
	sdkJobDB        sdkjob.Storage
	ciRunDB         cirun.Storage
	notificationDB  notificationdb.Storage
	orgDB           orgdb.Storage
	sdkConfig       config.SDKConfig
	protoLinter     *lint.Linter
	breakingChecker *breaking.Checker
	uow             db.UnitOfWork
	authz           *authorization.Server
	gitalyOpLog     *gitalyoplog.GitalyOpLogStorage
	logger          *log.LoggerWrapper
	registryHost    string
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB:        deps.ModuleDB,
		commitDB:        deps.CommitDB,
		gitStorage:      deps.GitStorage,
		sdkJobDB:        deps.SDKJobDB,
		ciRunDB:         deps.CIRunDB,
		notificationDB:  deps.NotificationDB,
		orgDB:           deps.OrgDB,
		sdkConfig:       deps.SDKConfig,
		protoLinter:     deps.ProtoLinter,
		breakingChecker: deps.BreakingChk,
		uow:             deps.UoW,
		authz:           deps.Authorization,
		gitalyOpLog:     deps.GitalyOpLog,
		logger:          deps.Logger,
		registryHost:    deps.RegistryHost,
	}
}
