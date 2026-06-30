// Package treesvc implements the TreeService ConnectRPC handler. It exposes
// directory listings and file content from module repositories, using the HEAD
// revision of each repository via the git.Storage abstraction.
package treesvc

import (
	"context"

	"connectrpc.com/connect"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler implements the TreeService ConnectRPC handler.
type Handler struct {
	registryv1connect.UnimplementedTreeServiceHandler

	logger     *log.LoggerWrapper
	gitStorage gitstorage.Storage
}

// NewHandler constructs a Handler from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		gitStorage: deps.GitStorage,
	}
}

// ListModuleFiles returns the depth-1 directory listing of a path inside the
// latest commit of the given owner/module repository.
func (h *Handler) ListModuleFiles(ctx context.Context, req *connect.Request[registryv1.ListModuleFilesRequest]) (*connect.Response[registryv1.ListModuleFilesResponse], error) {
	repoPath := req.Msg.Owner + "/" + req.Msg.Module
	gitEntries, err := h.gitStorage.GetTreeEntries(ctx, repoPath, "HEAD", req.Msg.Path)
	if err != nil {
		h.logger.Error("GetTreeEntries failed", "error", err, "owner", req.Msg.Owner, "module", req.Msg.Module, "path", req.Msg.Path)
		return nil, connErr.Internal(err.Error())
	}

	entries := make([]*registryv1.FileEntry, len(gitEntries))
	for i, e := range gitEntries {
		t := registryv1.FileEntryType_FILE_ENTRY_TYPE_FILE
		if e.Type == gitstorage.TreeEntryTypeDir {
			t = registryv1.FileEntryType_FILE_ENTRY_TYPE_DIR
		}
		entries[i] = &registryv1.FileEntry{
			Oid:  e.OID,
			Mode: e.Mode,
			Path: e.Path,
			Name: e.Name,
			Type: t,
		}
	}

	return &connect.Response[registryv1.ListModuleFilesResponse]{
		Msg: &registryv1.ListModuleFilesResponse{Entries: entries},
	}, nil
}

// GetFileContent returns the raw content of a single file identified by its
// path in the latest commit of the given owner/module repository.
func (h *Handler) GetFileContent(ctx context.Context, req *connect.Request[registryv1.GetFileContentRequest]) (*connect.Response[registryv1.GetFileContentResponse], error) {
	repoPath := req.Msg.Owner + "/" + req.Msg.Module
	content, size, err := h.gitStorage.GetFile(ctx, repoPath, "HEAD", req.Msg.Path)
	if err != nil {
		h.logger.Error("GetFileContent failed", "error", err, "owner", req.Msg.Owner, "module", req.Msg.Module, "path", req.Msg.Path)
		return nil, connErr.NotFound(err.Error())
	}

	return &connect.Response[registryv1.GetFileContentResponse]{
		Msg: &registryv1.GetFileContentResponse{
			Content: content,
			Size:    size,
		},
	}, nil
}
