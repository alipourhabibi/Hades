// Package module implements the ModuleService ConnectRPC handler. It handles
// module creation (including the initial Gitaly repository and commit), listing,
// and lookup. Module creation uses a unit-of-work with saga-style compensation:
// if the DB transaction fails after the Gitaly repository is created, the
// repository is deleted before returning the error.
package module

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

// moduleStorage is the subset of ModuleStorage used by the Server.
type moduleStorage interface {
	GetModulesByRefs(ctx context.Context, refs ...*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error)
	ListModules(ctx context.Context, ownerUsername string, limit, offset int) ([]*registrypbv1.Module, error)
	GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registrypbv1.Module, error)
	Create(ctx context.Context, name, ownerId string, visibility registrypbv1.ModuleVisibility, state registrypbv1.ModuleState, description, url, defaultLabelName, defaultBranch string, lintPreset registrypbv1.LintPreset, breakingEnabled bool) (*registrypbv1.Module, error)
	Update(ctx context.Context, req *registrypbv1.UpdateModuleRequest) (*registrypbv1.Module, error)
}

// authService is the subset of the authorization Server used by the Server.
type authService interface {
	CheckReadAccess(ctx context.Context, user *identityv1.User, modules []*registrypbv1.Module) error
	Can(ctx context.Context, in *constants.Policy) (*constants.CanResponse, error)
}

type Server struct {
	registryv1.ModuleServiceHandler

	logger          *log.LoggerWrapper
	registryHost    string
	moduleDBStorage moduleStorage
	commitDBStorage commitdb.Storage
	gitStorage      gitstorage.Storage
	authorization   authService
	uow             db.UnitOfWork
	gitalyOpLog     *gitalyoplog.GitalyOpLogStorage
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:          deps.Logger,
		registryHost:    deps.RegistryHost,
		moduleDBStorage: deps.ModuleDB,
		commitDBStorage: deps.CommitDB,
		gitStorage:      deps.GitStorage,
		authorization:   deps.Authorization,
		uow:             deps.UoW,
		gitalyOpLog:     deps.GitalyOpLog,
	}
}

// GetModules returns modules matching the given refs, enforcing read access for private ones.
// user may be nil (anonymous): public modules are returned; private ones produce NotFound.
func (s *Server) GetModules(ctx context.Context, refs []*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User) // nil for anonymous
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
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	pageSize := int(in.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if in.Msg.PageToken != "" {
		if n, err := strconv.Atoi(in.Msg.PageToken); err == nil {
			offset = n
		}
	}

	modules, err := s.moduleDBStorage.ListModules(ctx, in.Msg.Owner, pageSize, offset)
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

	nextPageToken := ""
	if len(modules) == pageSize {
		nextPageToken = strconv.Itoa(offset + pageSize)
	}

	return &connect.Response[registrypbv1.ListModulesResponse]{
		Msg: &registrypbv1.ListModulesResponse{Modules: visible, NextPageToken: nextPageToken},
	}, nil
}

func (s *Server) GetModule(ctx context.Context, in *connect.Request[registrypbv1.GetModuleRequest]) (*connect.Response[registrypbv1.GetModuleResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	m, err := s.moduleDBStorage.GetModuleByOwnerAndName(ctx, in.Msg.Owner, in.Msg.Name)
	if err != nil {
		userID := "anonymous"
		if user != nil {
			userID = user.Id
		}
		s.logger.Warn("module not found", "procedure", "GetModule", "user_id", userID, "owner", in.Msg.Owner, "name", in.Msg.Name, "error", err)
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

	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "CreateModuleByName")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	moduleFullName := user.Username + "/" + in.Msg.Name

	can, err := s.authorization.Can(ctx, &constants.Policy{
		Subject:      user.Username,
		ResourceType: string(constants.ResourceModule),
		Action:       string(constants.CREATE),
		Domain:       moduleFullName,
	})
	if err != nil {
		return nil, err
	}
	if !can.Allowed {
		s.logger.Warn("user not allowed to create module", "procedure", "CreateModuleByName", "user_id", user.Id, "module", moduleFullName)
		return nil, connErr.PermissionDenied("user is not allowed to create this repo")
	}

	lintPreset := in.Msg.LintPreset
	if lintPreset == registrypbv1.LintPreset_LINT_PRESET_UNSPECIFIED {
		lintPreset = registrypbv1.LintPreset_LINT_PRESET_DEFAULT
	}
	breakingEnabled := in.Msg.BreakingEnabled

	bsrName := moduleFullName
	if s.registryHost != "" {
		bsrName = s.registryHost + "/" + moduleFullName
	}
	bufYAML := fmt.Sprintf("version: v2\nmodules:\n  - path: .\n    name: %s\nlint:\n  use:\n    - %s\n", bsrName, lintPresetToRule(lintPreset))
	if breakingEnabled {
		bufYAML += "breaking:\n  use:\n    - FILE\n"
	}

	initialFiles := []*registrypbv1.File{
		{Path: "README.md", Content: []byte("")},
		{Path: "buf.yaml", Content: []byte(bufYAML)},
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
	result, err := s.uow.Do(ctx, func(ctx context.Context) (interface{}, error) {
		// 1. DB first: insert module row.
		module, err := s.moduleDBStorage.Create(
			ctx,
			moduleFullName,
			user.Id,
			in.Msg.Visibility,
			registrypbv1.ModuleState_MODULE_STATE_ACTIVE,
			in.Msg.Description,
			"",
			"",
			in.Msg.DefaultBranch,
			lintPreset,
			breakingEnabled,
		)
		if err != nil {
			return nil, connErr.FromPgx(err)
		}

		// 2. Git: create repository.
		if err := s.gitStorage.CreateRepository(ctx, moduleFullName, in.Msg.DefaultBranch); err != nil {
			return nil, err
		}

		// 3. Git: write initial commit.
		gitFiles := make([]*gitstorage.File, len(initialFiles))
		for i, f := range initialFiles {
			gitFiles[i] = &gitstorage.File{Path: f.Path, Content: f.Content}
		}
		commitHash, err := s.gitStorage.PutFiles(ctx, moduleFullName, in.Msg.DefaultBranch, gitFiles, user.Username, user.Email, "initial commit", nil)
		if err != nil {
			_ = s.gitStorage.DeleteRepository(ctx, moduleFullName)
			return nil, err
		}

		// 4. DB: insert commit row using the git-returned hash.
		if err := s.commitDBStorage.Create(
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
			_ = s.gitStorage.DeleteRepository(ctx, moduleFullName)
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

func (s *Server) UpdateModule(ctx context.Context, in *connect.Request[registrypbv1.UpdateModuleRequest]) (*connect.Response[registrypbv1.UpdateModuleResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "UpdateModule")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	moduleFullName := in.Msg.Owner + "/" + in.Msg.Name

	can, err := s.authorization.Can(ctx, &constants.Policy{
		Subject:      user.Username,
		ResourceType: string(constants.ResourceModule),
		Action:       string(constants.ActionUpdate),
		Domain:       moduleFullName,
	})
	if err != nil {
		return nil, err
	}
	if !can.Allowed {
		s.logger.Warn("user not allowed to update module", "procedure", "UpdateModule", "user_id", user.Id, "module", moduleFullName)
		return nil, connErr.PermissionDenied("user is not allowed to update this module")
	}

	updated, err := s.moduleDBStorage.Update(ctx, in.Msg)
	if err != nil {
		s.logger.Error("failed to update module", "error", err, "procedure", "UpdateModule", "module", moduleFullName)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[registrypbv1.UpdateModuleResponse]{
		Msg: &registrypbv1.UpdateModuleResponse{Module: updated},
	}, nil
}

func lintPresetToRule(p registrypbv1.LintPreset) string {
	switch p {
	case registrypbv1.LintPreset_LINT_PRESET_BASIC:
		return "BASIC"
	case registrypbv1.LintPreset_LINT_PRESET_MINIMAL:
		return "MINIMAL"
	case registrypbv1.LintPreset_LINT_PRESET_COMMENTS:
		return "COMMENTS"
	default:
		return "DEFAULT"
	}
}
