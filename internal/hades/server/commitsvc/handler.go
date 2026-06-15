// Package commitsvc implements the CommitService ConnectRPC handler.
// It exposes commit history for schema modules via two RPCs:
//   - ListCommits - returns all commits for a given owner/module, newest first.
//   - GetCommit   - returns a single commit by its git commit hash.
//
// Both RPCs enforce OPA read-access checks so that callers cannot retrieve
// commits belonging to private modules they are not authorised to read.
package commitsvc

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// readAccessChecker is the subset of the authorization Server used by Handler.
// Defined as an interface so that tests can substitute a fake.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registrypbv1.User, modules []*registrypbv1.Module) error
}

// Handler implements the CommitService ConnectRPC handler.
type Handler struct {
	registryv1connect.CommitServiceHandler

	logger          *log.LoggerWrapper
	commitDBStorage *commitdb.CommitStorage
	moduleDBStorage *moduledb.ModuleStorage
	authz           readAccessChecker
}

// NewHandler constructs a Handler wired to the storages and authorisation
// service from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		commitDBStorage: deps.CommitDB,
		moduleDBStorage: deps.ModuleDB,
		authz:           deps.Authorization,
	}
}

// ListCommits returns all commits for the module identified by owner and
// module name, ordered newest first.  Returns NOT_FOUND if the module does
// not exist.  Returns PERMISSION_DENIED (via CheckReadAccess) if the module
// is private and the caller is not authorised to read it.
func (h *Handler) ListCommits(ctx context.Context, in *connect.Request[registrypbv1.ListCommitsRequest]) (*connect.Response[registrypbv1.ListCommitsResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{
		Owner:  in.Msg.Owner,
		Module: in.Msg.Module,
	})
	if err != nil || len(modules) == 0 {
		h.logger.Warn("module not found", "procedure", "ListCommits", "user_id", userID, "owner", in.Msg.Owner, "module", in.Msg.Module)
		return nil, connErr.NotFound("module not found")
	}

	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	commits, err := h.commitDBStorage.ListByModule(ctx, modules[0].Id)
	if err != nil {
		h.logger.Error("failed to list commits", "error", err, "procedure", "ListCommits", "user_id", userID, "module_id", modules[0].Id)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[registrypbv1.ListCommitsResponse]{
		Msg: &registrypbv1.ListCommitsResponse{Commits: commits},
	}, nil
}

// GetCommit returns the commit with the given hash.  After fetching the
// commit the handler looks up the owning module and calls CheckReadAccess,
// so callers cannot enumerate commits from private modules they cannot read.
func (h *Handler) GetCommit(ctx context.Context, in *connect.Request[registrypbv1.GetCommitRequest]) (*connect.Response[registrypbv1.GetCommitResponse], error) {
	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	commit, err := h.commitDBStorage.GetByHash(ctx, in.Msg.CommitHash)
	if err != nil {
		h.logger.Warn("commit not found", "error", err, "procedure", "GetCommit", "user_id", userID, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.NotFound("commit not found")
	}

	// Check read access on the module this commit belongs to.
	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{Id: commit.ModuleId})
	if err != nil || len(modules) == 0 {
		h.logger.Warn("module not found for commit", "procedure", "GetCommit", "user_id", userID, "module_id", commit.ModuleId)
		return nil, connErr.NotFound("module not found")
	}
	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	return &connect.Response[registrypbv1.GetCommitResponse]{
		Msg: &registrypbv1.GetCommitResponse{Commit: commit},
	}, nil
}
