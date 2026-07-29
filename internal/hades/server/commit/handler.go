// Package commit implements the CommitService ConnectRPC handler.
// It exposes commit history, file diffs, and tree browsing for schema modules:
//   - ListCommits     - returns all commits for a given owner/module, newest first.
//   - GetCommit       - returns a single commit by its git commit hash.
//   - GetCommitDiff   - returns per-file diffs for a commit vs its parent.
//   - ListModuleFiles - returns the depth-1 directory listing of a path.
//   - GetFileContent  - returns the raw content of a single file.
//
// All RPCs enforce OPA read-access checks so callers cannot retrieve data
// from private modules they are not authorised to read.
package commit

import (
	"context"

	"connectrpc.com/connect"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *identityv1.User, modules []*registrypbv1.Module) error
}

// Handler implements the CommitService ConnectRPC handler.
type Handler struct {
	registryv1connect.UnimplementedCommitServiceHandler

	logger          *log.LoggerWrapper
	commitDBStorage commitdb.Storage
	moduleDBStorage moduledb.Storage
	gitStorage      gitstorage.Storage
	authz           readAccessChecker
}

// NewHandler constructs a Handler wired to the storages and authorisation
// service from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:          deps.Logger,
		commitDBStorage: deps.CommitDB,
		moduleDBStorage: deps.ModuleDB,
		gitStorage:      deps.GitStorage,
		authz:           deps.Authorization,
	}
}

func (h *Handler) ListCommits(ctx context.Context, in *connect.Request[registrypbv1.ListCommitsRequest]) (*connect.Response[registrypbv1.ListCommitsResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

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

func (h *Handler) GetCommit(ctx context.Context, in *connect.Request[registrypbv1.GetCommitRequest]) (*connect.Response[registrypbv1.GetCommitResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	userID := "anonymous"
	if user != nil {
		userID = user.Id
	}

	commit, err := h.commitDBStorage.GetByHash(ctx, in.Msg.CommitHash)
	if err != nil {
		h.logger.Warn("commit not found", "error", err, "procedure", "GetCommit", "user_id", userID, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.NotFound("commit not found")
	}

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

func (h *Handler) GetCommitDiff(ctx context.Context, in *connect.Request[registrypbv1.GetCommitDiffRequest]) (*connect.Response[registrypbv1.GetCommitDiffResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

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

	fileDiffs, err := h.gitStorage.GetCommitDiff(ctx, modules[0].Name, in.Msg.CommitHash)
	if err != nil {
		h.logger.Error("failed to get commit diff", "error", err, "procedure", "GetCommitDiff", "user_id", userID, "commit_hash", in.Msg.CommitHash)
		return nil, connErr.Internal("failed to get commit diff")
	}

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
			Diffs:          protoDiffs,
			TotalAdditions: totalAdditions,
			TotalDeletions: totalDeletions,
		},
	}, nil
}

func (h *Handler) ListModuleFiles(ctx context.Context, req *connect.Request[registrypbv1.ListModuleFilesRequest]) (*connect.Response[registrypbv1.ListModuleFilesResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{
		Owner:  req.Msg.Owner,
		Module: req.Msg.Module,
	})
	if err != nil || len(modules) == 0 {
		return nil, connErr.NotFound("module not found")
	}
	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	repoPath := req.Msg.Owner + "/" + req.Msg.Module
	gitEntries, err := h.gitStorage.GetTreeEntries(ctx, repoPath, "HEAD", req.Msg.Path)
	if err != nil {
		h.logger.Error("GetTreeEntries failed", "error", err, "owner", req.Msg.Owner, "module", req.Msg.Module, "path", req.Msg.Path)
		return nil, connErr.Internal("failed to list files")
	}

	entries := make([]*registrypbv1.FileEntry, len(gitEntries))
	for i, e := range gitEntries {
		t := registrypbv1.FileEntryType_FILE_ENTRY_TYPE_FILE
		if e.Type == gitstorage.TreeEntryTypeDir {
			t = registrypbv1.FileEntryType_FILE_ENTRY_TYPE_DIR
		}
		entries[i] = &registrypbv1.FileEntry{
			Oid:  e.OID,
			Mode: e.Mode,
			Path: e.Path,
			Name: e.Name,
			Type: t,
		}
	}

	return &connect.Response[registrypbv1.ListModuleFilesResponse]{
		Msg: &registrypbv1.ListModuleFilesResponse{Entries: entries},
	}, nil
}

func (h *Handler) GetFileContent(ctx context.Context, req *connect.Request[registrypbv1.GetFileContentRequest]) (*connect.Response[registrypbv1.GetFileContentResponse], error) {
	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	modules, err := h.moduleDBStorage.GetModulesByRefs(ctx, &registrypbv1.ModuleRef{
		Owner:  req.Msg.Owner,
		Module: req.Msg.Module,
	})
	if err != nil || len(modules) == 0 {
		return nil, connErr.NotFound("module not found")
	}
	if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
		return nil, err
	}

	repoPath := req.Msg.Owner + "/" + req.Msg.Module
	content, size, err := h.gitStorage.GetFile(ctx, repoPath, "HEAD", req.Msg.Path)
	if err != nil {
		h.logger.Error("GetFileContent failed", "error", err, "owner", req.Msg.Owner, "module", req.Msg.Module, "path", req.Msg.Path)
		return nil, connErr.NotFound("file not found")
	}

	return &connect.Response[registrypbv1.GetFileContentResponse]{
		Msg: &registrypbv1.GetFileContentResponse{
			Content: content,
			Size:    size,
		},
	}, nil
}
