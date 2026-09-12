// Package module implements the ModuleService ConnectRPC handler. It handles
// module creation (including the initial Gitaly repository and commit), listing,
// and lookup. Module creation uses a unit-of-work with saga-style compensation:
// if the DB transaction fails after the Gitaly repository is created, the
// repository is deleted before returning the error.
package module

import (
	"context"
	"errors"
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
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

// recordOpLogFailure marks a pending Gitaly operation-log entry as failed.
// Best effort: the log exists so a crashed operation can be compensated later,
// and a failure to update it must not mask the original error.
func (s *Server) recordOpLogFailure(ctx context.Context, logID uuid.UUID, cause error) {
	if s.gitalyOpLog == nil || logID == uuid.Nil {
		return
	}
	if err := s.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", cause.Error()); err != nil {
		s.logger.Error("failed to update gitaly op log", "error", err, "log_id", logID)
	}
}

// orgStorage is the subset of org.Storage used by the Server.
type orgStorage interface {
	GetByName(ctx context.Context, name string) (*identityv1.User, error)
}

// moduleStorage is the subset of ModuleStorage used by the Server.
type moduleStorage interface {
	GetModulesByRefs(ctx context.Context, refs ...*registrypbv1.ModuleRef) ([]*registrypbv1.Module, error)
	ListVisibleModules(ctx context.Context, ownerUsername, subject, subjectID string, limit, offset int) ([]*registrypbv1.Module, error)
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
	registryv1.UnimplementedModuleServiceHandler

	logger          *log.LoggerWrapper
	registryHost    string
	moduleDBStorage moduleStorage
	commitDBStorage commitdb.Storage
	gitStorage      gitstorage.Storage
	authorization   authService
	orgDBStorage    orgStorage
	uow             db.UnitOfWork
	gitalyOpLog     gitalyoplog.Storage
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:          deps.Logger,
		registryHost:    deps.RegistryHost,
		moduleDBStorage: deps.ModuleDB,
		commitDBStorage: deps.CommitDB,
		gitStorage:      deps.GitStorage,
		authorization:   deps.Authorization,
		orgDBStorage:    deps.OrgDB,
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
	// Nothing is returned alongside the error. Returning the modules and the
	// access error together made correctness depend on every caller checking the
	// error first, and one call site that did not would disclose private modules.
	if err := s.authorization.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}
	return modules, nil
}

func (s *Server) ListModules(ctx context.Context, in *connect.Request[registrypbv1.ListModulesRequest]) (*connect.Response[registrypbv1.ListModulesResponse], error) {
	// user may be nil when called without an Authorization header (anonymous access).
	// Anonymous callers receive only public modules; authenticated callers receive
	// public modules plus any private modules they are authorised to read.
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	pageSize, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	// The query itself narrows to what the caller can plausibly read: public
	// modules, their own, and anything covered by one of their role bindings.
	// CheckReadAccess still runs on every row because the OPA policy, not the
	// SQL, is the authority; the pre-filter exists so that a page is not mostly
	// discarded afterwards.
	//
	// The batch loop remains as the backstop for the rare row the pre-filter
	// admits but the policy rejects, so a page is still the size the caller
	// asked for. In practice it now completes on the first iteration.
	subject, subjectID := "", ""
	if user != nil {
		subject, subjectID = user.Username, user.Id
	}

	visible := make([]*registrypbv1.Module, 0, pageSize)
	scanOffset := offset
	exhausted := false
	for batch := 0; batch < maxListScanBatches && len(visible) < pageSize; batch++ {
		modules, err := s.moduleDBStorage.ListVisibleModules(ctx, in.Msg.Owner, subject, subjectID, pageSize, scanOffset)
		if err != nil {
			s.logger.Error("failed to list modules", "error", err, "procedure", "ListModules", "user_id", userID)
			return nil, connerr.FromDB(err)
		}
		if len(modules) == 0 {
			exhausted = true
			break
		}
		for _, m := range modules {
			scanOffset++
			// CheckReadAccess reports private modules the caller cannot read as
			// NotFound, which is what hides their existence. List semantics are
			// to filter rather than fail on the first denied module.
			if err := s.authorization.CheckReadAccess(ctx, user, []*registrypbv1.Module{m}); err == nil {
				visible = append(visible, m)
				if len(visible) == pageSize {
					break
				}
			}
		}
		if len(modules) < pageSize {
			exhausted = true
			break
		}
	}

	nextPageToken := ""
	if !exhausted {
		nextPageToken = strconv.Itoa(scanOffset)
	}

	return &connect.Response[registrypbv1.ListModulesResponse]{
		Msg: &registrypbv1.ListModulesResponse{Modules: visible, NextPageToken: nextPageToken},
	}, nil
}

// maxListScanBatches bounds how many raw pages ListModules will read while
// trying to fill one visible page. The SQL pre-filter normally makes one batch
// enough; this bounds the pathological case where the policy rejects rows the
// pre-filter admitted.
const maxListScanBatches = 10

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
		return nil, connerr.NotFound("module not found")
	}

	if err := s.authorization.CheckReadAccess(ctx, user, []*registrypbv1.Module{m}); err != nil {
		// Surface as not-found so as not to leak existence of private modules.
		return nil, connerr.NotFound("module not found")
	}

	return &connect.Response[registrypbv1.GetModuleResponse]{
		Msg: &registrypbv1.GetModuleResponse{Module: m},
	}, nil
}

func (s *Server) CreateModuleByName(ctx context.Context, in *connect.Request[registrypbv1.CreateModuleByNameRequest]) (*connect.Response[registrypbv1.CreateModuleByNameResponse], error) {

	in.Msg.Name = strings.ToLower(strings.TrimSpace(in.Msg.Name))
	// Before any storage or git call. The module name becomes the second
	// segment of every URL the module appears at and the second segment of its
	// repository path on disk, and neither was checked: "../escape" created a
	// repository outside the owner's namespace and "has space" created a
	// database row with no repository behind it. See constants.ValidateName.
	if err := constants.ValidateName(in.Msg.Name); err != nil {
		return nil, connerr.InvalidArgument("module " + err.Error())
	}
	if in.Msg.DefaultBranch == "" {
		in.Msg.DefaultBranch = "main"
	}

	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "CreateModuleByName")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	// The namespace defaults to the caller's own username. Naming an
	// organisation here creates the module in that org's namespace, which the
	// OPA check below then authorises: org membership grants bindings over
	// "<org>/*", so no policy change is needed to support it.
	ownerName := strings.ToLower(strings.TrimSpace(in.Msg.Owner))
	if ownerName == "" {
		ownerName = user.Username
	}
	ownerID := user.Id
	if ownerName != user.Username {
		org, err := s.orgDBStorage.GetByName(ctx, ownerName)
		if err != nil {
			s.logger.Warn("module owner namespace not found", "procedure", "CreateModuleByName", "user_id", user.Id, "owner", ownerName)
			return nil, connerr.NotFound("owner namespace not found")
		}
		ownerID = org.Id
	}

	moduleFullName := ownerName + "/" + in.Msg.Name

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
		return nil, connerr.PermissionDenied("user is not allowed to create this repo")
	}

	// Answer "this name is taken" before doing any git work.
	//
	// Creating the same module twice used to return Internal. The unique
	// constraint is on the database row, and the git steps run first: the
	// repository already existed, CreateRepository is idempotent, and PutFiles
	// then refused with ErrRefMoved because it expected no branch. That error
	// reached the interceptor untranslated and was flattened, so a client could
	// not tell "this name is taken" from "the server is broken" and a retry
	// loop retried something that would never succeed.
	//
	// The database constraint is still the authority: this is a check, not a
	// lock, and two concurrent creates can both pass it. The one that loses
	// gets AlreadyExists from connerr.FromDB inside the transaction below.
	if existing, err := s.moduleDBStorage.GetModuleByOwnerAndName(ctx, ownerName, in.Msg.Name); err == nil && existing != nil {
		return nil, connerr.AlreadyExists("module " + moduleFullName + " already exists")
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

	// Git work first, then a short DB transaction, matching the ordering settled
	// for Upload: a Gitaly RPC inside an open transaction holds a database
	// connection across two network round trips.
	//
	// Order: (1) Gitaly CreateRepository → (2) Gitaly UserCommitFiles →
	//        (3) one transaction inserting the module and commit rows.
	//
	// Compensation is saga-style: if any later step fails, the repository
	// created in (1) is deleted. The pending gitalyOpLog entry written above
	// lets the cleanup job compensate if the process dies mid-sequence.
	if err := s.gitStorage.CreateRepository(ctx, moduleFullName, in.Msg.DefaultBranch); err != nil {
		s.recordOpLogFailure(ctx, logID, err)
		return nil, err
	}

	gitFiles := make([]*gitstorage.File, len(initialFiles))
	for i, f := range initialFiles {
		gitFiles[i] = &gitstorage.File{Path: f.Path, Content: f.Content}
	}
	// ExpectedHead is empty: this is the first commit in a repository that was
	// created a few lines above, so the branch must not exist yet. If it does,
	// something else raced us and PutFiles refuses rather than overwriting.
	commitHash, err := s.gitStorage.PutFiles(ctx, gitstorage.PutFilesRequest{
		RepoPath:    moduleFullName,
		Branch:      in.Msg.DefaultBranch,
		Files:       gitFiles,
		AuthorName:  user.Username,
		AuthorEmail: user.Email,
		Message:     "initial commit",
	})
	if err != nil {
		// The repository is not deleted when the branch already existed: it
		// belongs to a module someone else created, and removing it would turn
		// a duplicate-name error into data loss.
		if errors.Is(err, gitstorage.ErrRefMoved) {
			s.recordOpLogFailure(ctx, logID, err)
			return nil, connerr.AlreadyExists("module " + moduleFullName + " already exists")
		}
		_ = s.gitStorage.DeleteRepository(ctx, moduleFullName)
		s.recordOpLogFailure(ctx, logID, err)
		return nil, err
	}

	result, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		module, err := s.moduleDBStorage.Create(
			txCtx,
			moduleFullName,
			ownerID,
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
			return nil, connerr.FromDB(err)
		}

		if err := s.commitDBStorage.Create(
			txCtx,
			uuid.New(),
			commitHash,
			user.Id,
			module.Id,
			registrypbv1.DigestType_DIGEST_TYPE_B5,
			"",
			user.Id,
			"",
		); err != nil {
			return nil, connerr.FromDB(err)
		}

		return module, nil
	}, 30*time.Second)
	if err != nil {
		// The DB rolled itself back; undo the git side to match. A failure to
		// compensate is logged rather than dropped: it leaves an orphan
		// repository that will collide with the next attempt to create a module
		// with this name, and the failure would otherwise surface there, far
		// from its cause.
		if delErr := s.gitStorage.DeleteRepository(ctx, moduleFullName); delErr != nil {
			s.logger.Error("failed to remove the repository after a failed module creation",
				"error", delErr, "procedure", "CreateModule", "module", moduleFullName,
				"cause", err)
			if s.gitalyOpLog != nil && logID != uuid.Nil {
				_ = s.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "",
					"module creation failed and the repository could not be removed: "+delErr.Error())
			}
		}
	}

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

	module, ok := result.(*registrypbv1.Module)
	if !ok {
		s.logger.Error("unexpected result type from the create-module transaction",
			"procedure", "CreateModuleByName", "module", moduleFullName)
		return nil, connerr.Internal("failed to create module")
	}

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
		return nil, connerr.Unauthenticated("not authenticated")
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
		return nil, connerr.PermissionDenied("user is not allowed to update this module")
	}

	// Making a private module public is checked separately.
	//
	// module:update covers editing a description; publishing a private schema
	// to the world is a materially different decision, and a contributor who
	// legitimately holds update rights should not necessarily hold it. Only the
	// private-to-public direction needs the extra permission: making a public
	// module private removes access rather than granting it.
	if in.Msg.Visibility != nil && *in.Msg.Visibility == registrypbv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC {
		current, err := s.moduleDBStorage.GetModuleByOwnerAndName(ctx, in.Msg.Owner, in.Msg.Name)
		if err != nil {
			s.logger.Error("failed to read the module before a visibility change", "error", err,
				"procedure", "UpdateModule", "module", moduleFullName)
			return nil, connerr.FromDB(err)
		}
		if current.Visibility != registrypbv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC {
			canPublish, err := s.authorization.Can(ctx, &constants.Policy{
				Subject:      user.Username,
				ResourceType: string(constants.ResourceModule),
				Action:       string(constants.ActionPublish),
				Domain:       moduleFullName,
			})
			if err != nil {
				return nil, err
			}
			if !canPublish.Allowed {
				s.logger.Warn("user not allowed to publish module", "procedure", "UpdateModule",
					"user_id", user.Id, "module", moduleFullName)
				return nil, connerr.PermissionDenied("user is not allowed to make this module public")
			}
		}
	}

	updated, err := s.moduleDBStorage.Update(ctx, in.Msg)
	if err != nil {
		s.logger.Error("failed to update module", "error", err, "procedure", "UpdateModule", "module", moduleFullName)
		return nil, connerr.FromDB(err)
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
