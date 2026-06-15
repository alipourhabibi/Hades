// Package diffsvc implements the DiffService ConnectRPC handler.
// It exposes commit diffs via one RPC:
//   - GetCommitDiff - returns per-file diffs for a commit vs its parent.
//
// The RPC enforces the same read-access checks as CommitService so that
// callers cannot retrieve diffs from private modules they are not authorised
// to read.
package diffsvc

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	gitaly_diff "github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/diff"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// readAccessChecker is the subset of the authorization Server used by Handler.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registrypbv1.User, modules []*registrypbv1.Module) error
}

// Handler implements the DiffService ConnectRPC handler.
type Handler struct {
	registryv1connect.DiffServiceHandler

	logger          *log.LoggerWrapper
	commitDBStorage *commitdb.CommitStorage
	moduleDBStorage *moduledb.ModuleStorage
	gitalyDiff      *gitaly_diff.DiffService
	authz           readAccessChecker
}

// NewHandler constructs a Handler wired to the storages and authorisation
// service from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		commitDBStorage: deps.CommitDB,
		moduleDBStorage: deps.ModuleDB,
		gitalyDiff:      deps.GitalyDiffStorage,
		authz:           deps.Authorization,
	}
}

// GetCommitDiff returns the per-file diffs for the commit identified by
// CommitHash.  Returns NOT_FOUND if the commit or its module do not exist.
// Returns PERMISSION_DENIED (via CheckReadAccess) if the module is private and
// the caller is not authorised to read it.
func (h *Handler) GetCommitDiff(ctx context.Context, in *connect.Request[registrypbv1.GetCommitDiffRequest]) (*connect.Response[registrypbv1.GetCommitDiffResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	commit, err := h.commitDBStorage.GetByHash(ctx, in.Msg.CommitHash)
	if err != nil {
		h.logger.Warn("commit not found", "error", err, "procedure", "GetCommitDiff", "user_id", userID, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.NotFound("commit not found")
	}

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{Id: commit.ModuleId})
	if err != nil || len(modules) == 0 {
		h.logger.Warn("module not found for commit", "procedure", "GetCommitDiff", "user_id", userID, "module_id", commit.ModuleId)
		return nil, connErr.NotFound("module not found")
	}

	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	// Module name is stored as "{owner}/{module}".
	parts := strings.SplitN(modules[0].Name, "/", 2)
	if len(parts) != 2 {
		h.logger.Error("unexpected module name format", "procedure", "GetCommitDiff", "name", modules[0].Name)
		return nil, connErr.Internal("unexpected module name format")
	}
	owner, moduleName := parts[0], parts[1]

	fileDiffs, err := h.gitalyDiff.GetCommitDiff(ctx, owner, moduleName, in.Msg.CommitHash)
	if err != nil {
		h.logger.Error("failed to get commit diff", "error", err, "procedure", "GetCommitDiff", "user_id", userID, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.Internal("failed to get commit diff")
	}

	// Convert to proto and compute totals.
	protoDiffs := make([]*registrypbv1.FileDiff, 0, len(fileDiffs))
	var totalAdditions, totalDeletions int32
	for _, fd := range fileDiffs {
		protoDiffs = append(protoDiffs, &registrypbv1.FileDiff{
			FromPath:      fd.FromPath,
			ToPath:        fd.ToPath,
			IsNewFile:     fd.IsNewFile,
			IsDeletedFile: fd.IsDeletedFile,
			IsRenamedFile: fd.IsRenamedFile,
			Additions:     fd.Additions,
			Deletions:     fd.Deletions,
			Patch:         fd.Patch,
			Binary:        fd.Binary,
			TooLarge:      fd.TooLarge,
		})
		totalAdditions += fd.Additions
		totalDeletions += fd.Deletions
	}

	return &connect.Response[registrypbv1.GetCommitDiffResponse]{
		Msg: &registrypbv1.GetCommitDiffResponse{
			Diffs:            protoDiffs,
			TotalAdditions:   totalAdditions,
			TotalDeletions:   totalDeletions,
		},
	}, nil
}
