// Package content consolidates upload and download logic.
package content

import (
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
)

// Handler handles both upload and download operations.
type Handler struct {
	moduleDB        moduledb.Storage
	commitDB        commitdb.Storage
	gitStorage      gitstorage.Storage
	sdkJobDB        sdkjob.Storage
	sdkConfig       config.SDKConfig
	protoLinter     *lint.Linter
	breakingChecker *breaking.Checker
	uow             db.UnitOfWork
	authz           *authorization.Server
	gitalyOpLog     *gitalyoplog.GitalyOpLogStorage
	registryHost    string
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB:        deps.ModuleDB,
		commitDB:        deps.CommitDB,
		gitStorage:      deps.GitStorage,
		sdkJobDB:        deps.SDKJobDB,
		sdkConfig:       deps.SDKConfig,
		protoLinter:     deps.ProtoLinter,
		breakingChecker: deps.BreakingChk,
		uow:             deps.UoW,
		authz:           deps.Authorization,
		gitalyOpLog:     deps.GitalyOpLog,
		registryHost:    deps.RegistryHost,
	}
}
