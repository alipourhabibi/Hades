// Package module implements the ModuleService ConnectRPC handler. It handles
// module creation (including the initial Gitaly repository and commit), listing,
// and lookup. Module creation uses a unit-of-work with saga-style compensation:
// if the DB transaction fails after the Gitaly repository is created, the
// repository is deleted before returning the error.
package module

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// moduleStorage is the subset of ModuleStorage used by the Server.
type moduleStorage interface {
	GetModulesByRefs(ctx context.Context, refs ...*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error)
	ListModules(ctx context.Context, ownerUsername string) ([]*registrypbv1.Module, error)
	GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registrypbv1.Module, error)
	WithTx(tx pgx.Tx) *moduledb.ModuleStorage
}

// authService is the subset of the authorization Server used by the Server.
type authService interface {
	CheckReadAccess(ctx context.Context, user *registrypbv1.User, modules []*registrypbv1.Module) error
	Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error)
	AddBasicRolesInTx(ctx context.Context, tx pgx.Tx, userName string) error
	ReloadPolicy() error
}

type Server struct {
	registryv1.ModuleServiceHandler

	logger                  *log.LoggerWrapper
	moduleDBStorage         moduleStorage
	commitDBStorage         *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorization           authService
	blobStorage             *blob.BlobService
	uow                     db.UnitOfWork
	gitalyOpLog             *gitalyoplog.GitalyOpLogStorage
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:                  deps.Logger,
		moduleDBStorage:         deps.ModuleDB,
		commitDBStorage:         deps.CommitDB,
		gitalyRepositoryService: deps.GitalyRepositoryStorage,
		gitalyOperationService:  deps.GitalyOperationStorage,
		authorization:           deps.Authorization,
		blobStorage:             deps.GitalyBlobStorage,
		uow:                     deps.UoW,
		gitalyOpLog:             deps.GitalyOpLog,
	}
}

// GetModules returns modules matching the given refs, enforcing read access for private ones.
// user may be nil (anonymous): public modules are returned; private ones produce NotFound.
func (s *Server) GetModules(ctx context.Context, refs []*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User) // nil for anonymous
	modules, err := s.moduleDBStorage.GetModulesByRefs(ctx, refs...)
	if err != nil {
		return nil, err
	}
	return modules, s.authorization.CheckReadAccess(ctx, user, modules)
}

func (s *Server) ListModules(ctx context.Context, in *connect.Request[registrypbv1.ListModulesRequest]) (*connect.Response[registrypbv1.ListModulesResponse], error) {
	// user may be nil when called without an Authorization header (anonymous access).
	// Anonymous callers receive only public modules; authenticated callers receive
	// public modules plus any private modules they are authorised to read.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

	modules, err := s.moduleDBStorage.ListModules(ctx, in.Msg.Owner)
	if err != nil {
		userID := "anonymous"
		if user != nil {
			userID = user.Id
		}
		s.logger.Error("failed to list modules", "error", err, "procedure", "ListModules", "user_id", userID)
		return nil, connErr.FromPgx(err)
	}

	// Filter to only modules the caller can read. CheckReadAccess silently
	// returns NotFound for private modules the caller cannot access - that
	// error code is used to hide their existence from anonymous callers.
	// List semantics: filter rather than fail on the first denied module.
	var visible []*registrypbv1.Module
	for _, m := range modules {
		if err := s.authorization.CheckReadAccess(ctx, user, []*registrypbv1.Module{m}); err == nil {
			visible = append(visible, m)
		}
	}

	return &connect.Response[registrypbv1.ListModulesResponse]{
		Msg: &registrypbv1.ListModulesResponse{Modules: visible},
	}, nil
}

func (s *Server) GetModule(ctx context.Context, in *connect.Request[registrypbv1.GetModuleRequest]) (*connect.Response[registrypbv1.GetModuleResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

	m, err := s.moduleDBStorage.GetModuleByOwnerAndName(ctx, in.Msg.Owner, in.Msg.Name)
	if err != nil {
		userID := "anonymous"
		if user != nil {
			userID = user.Id
		}
		s.logger.Warn("module not found", "procedure", "GetModule", "user_id", userID, "owner", in.Msg.Owner, "name", in.Msg.Name)
		return nil, connErr.NotFound("module not found")
	}

	if err := s.authorization.CheckReadAccess(ctx, user, []*registrypbv1.Module{m}); err != nil {
		// Surface as not-found so as not to leak existence of private modules.
		return nil, connErr.NotFound("module not found")
	}

	return &connect.Response[registrypbv1.GetModuleResponse]{
		Msg: &registrypbv1.GetModuleResponse{Module: m},
	}, nil
}

func (s *Server) CreateModuleByName(ctx context.Context, in *connect.Request[registrypbv1.CreateModuleByNameRequest]) (*connect.Response[registrypbv1.CreateModuleByNameResponse], error) {

	in.Msg.Name = strings.ToLower(in.Msg.Name)
	if in.Msg.DefaultBranch == "" {
		in.Msg.DefaultBranch = "main"
	}

	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "CreateModuleByName")
		return nil, connErr.Internal("missing user in context")
	}

	moduleFullName := user.Username + "/" + in.Msg.Name

	can, err := s.authorization.Can(ctx, &constants.Policy{
		Subject: user.Username,
		Object:  string(constants.REPOSITORY),
		Action:  string(constants.CREATE),
		Domain:  moduleFullName,
	})
	if err != nil {
		return nil, err
	}
	if !can.Allowed {
		s.logger.Warn("user not allowed to create module", "procedure", "CreateModuleByName", "user_id", user.Id, "module", moduleFullName)
		return nil, connErr.PermissionDenied("user is not allowed to create this repo")
	}

	// gitalyModule is a lightweight stub used only for Gitaly RPC calls.
	gitalyModule := &registrypbv1.Module{
		OwnerId:       user.Id,
		Name:          moduleFullName,
		DefaultBranch: in.Msg.DefaultBranch,
	}

	initialFiles := []*registrypbv1.File{
		{Path: "README.md", Content: []byte("")},
	}
	digestValue, err := shake256.DigestFiles(initialFiles)
	if err != nil {
		return nil, err
	}

	// Write a 'pending' log entry (auto-committed, outside the UoW) so that the
	// background cleanup job can compensate if the server crashes mid-operation.
	var logID uuid.UUID
	if s.gitalyOpLog != nil {
		logID, _ = s.gitalyOpLog.CreatePending(ctx, gitalyoplog.OpCreateModule, moduleFullName, user.Id)
	}

	// All DB and Gitaly operations happen inside a single UoW callback.
	//
	// Order: (1) DB module insert → (2) Gitaly CreateRepository →
	//        (3) Gitaly UserCommitFiles → (4) DB commit insert.
	//
	// On any error inside the callback:
	//   - The UoW auto-rolls back all DB writes.
	//   - Any Gitaly repository created is removed via DeleteRepository.
	result, err := s.uow.Do(ctx, func(ctx context.Context, tx pgx.Tx) (interface{}, error) {
		// 1. DB first: insert module row.
		module, err := s.moduleDBStorage.WithTx(tx).Create(
			ctx,
			moduleFullName,
			user.Id,
			registrypbv1.ModuleVisibility(in.Msg.Visibility),
			registrypbv1.ModuleState(registrypbv1.EState_E_STATE_ACTIVE),
			in.Msg.Description,
			"",
			"",
			in.Msg.DefaultBranch,
		)
		if err != nil {
			return nil, connErr.FromPgx(err)
		}

		// 2. Gitaly: create repository.
		// Failure here triggers DB auto-rollback; no Gitaly compensation needed
		// since CreateRepository itself failed.
		if err := s.gitalyRepositoryService.CreateRepository(ctx, gitalyModule); err != nil {
			return nil, err
		}

		// 3. Gitaly: write initial commit.
		// Compensation: delete the repository just created in step 2.
		commitHash, err := s.gitalyOperationService.UserCommitFiles(ctx, gitalyModule, initialFiles, user, []string{}, digestValue.String())
		if err != nil {
			_ = s.gitalyRepositoryService.DeleteRepository(ctx, gitalyModule)
			return nil, err
		}

		// 4. DB: insert commit row using the Gitaly-returned hash.
		// Compensation: delete the repository (and its commit) created above.
		if err := s.commitDBStorage.WithTx(tx).Create(
			ctx,
			uuid.New(),
			commitHash,
			user.Id,
			module.Id,
			registrypbv1.DigestType_DIGEST_TYPE_B5,
			"",
			user.Id,
			"",
		); err != nil {
			_ = s.gitalyRepositoryService.DeleteRepository(ctx, gitalyModule)
			return nil, connErr.FromPgx(err)
		}

		return module, nil
	}, 30*time.Second)

	// Update the operation log (auto-committed, outside the UoW).
	if s.gitalyOpLog != nil && logID != uuid.Nil {
		status := gitalyoplog.StatusCompleted
		errReason := ""
		if err != nil {
			status = gitalyoplog.StatusFailed
			errReason = err.Error()
		}
		_ = s.gitalyOpLog.UpdateStatus(ctx, logID, status, "", errReason)
	}

	if err != nil {
		return nil, err
	}

	module := result.(*registrypbv1.Module)

	return &connect.Response[registrypbv1.CreateModuleByNameResponse]{
		Msg: &registrypbv1.CreateModuleByNameResponse{
			Module: module,
		},
	}, nil
}
