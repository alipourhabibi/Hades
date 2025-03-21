package server

import (
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	sessiondb "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/casbin/casbin/v2"
)

type Dependencies struct {
	// ModuleServer         *module.Server
	// BufModuleServer      *bufmodules.Server
	// BufCommitServer      *bufcommits.Server
	// BufUploadServer      *upload.Server
	// BufGraphServer       *bufgraph.Server
	// BufDownloadServer    *bufdownload.Server

	CasbinEnforcer          *casbin.Enforcer
	ModuleDB                *moduledb.ModuleStorage
	CommitDB                *commitdb.CommitStorage
	UserDB                  *userdb.UserStorage
	SessionDB               *sessiondb.SessionStorage
	GitalyBlobStorage       *blob.BlobService
	GitalyRepositoryStorage *repository.RepositoryService
	GitalyOperationStorage  *operation.OperationService
	Authorization           *authorization.Server

	Logger *log.LoggerWrapper
}
