// Package treesvc implements the TreeService ConnectRPC handler. It exposes
// directory listings and file content from module repositories stored in
// Gitaly, using the HEAD revision of each repository.
package treesvc

import (
	"context"

	"connectrpc.com/connect"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	gitaly_tree "github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/tree"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler implements the TreeService ConnectRPC handler.
type Handler struct {
	registryv1connect.UnimplementedTreeServiceHandler

	logger      *log.LoggerWrapper
	treeStorage *gitaly_tree.TreeService
}

// NewHandler constructs a Handler from the shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:      deps.Logger,
		treeStorage: deps.GitalyTreeStorage,
	}
}

// ListModuleFiles returns the depth-1 directory listing of a path inside the
// latest commit of the given owner/module repository.
func (h *Handler) ListModuleFiles(ctx context.Context, req *connect.Request[registryv1.ListModuleFilesRequest]) (*connect.Response[registryv1.ListModuleFilesResponse], error) {
	entries, err := h.treeStorage.GetTreeEntries(ctx, req.Msg.Owner, req.Msg.Module, req.Msg.Path)
	if err != nil {
		h.logger.Error("GetTreeEntries failed", "error", err, "owner", req.Msg.Owner, "module", req.Msg.Module, "path", req.Msg.Path)
		return nil, connErr.Internal(err.Error())
	}

	return &connect.Response[registryv1.ListModuleFilesResponse]{
		Msg: &registryv1.ListModuleFilesResponse{Entries: entries},
	}, nil
}

// GetFileContent returns the raw content of a single file identified by its
// path in the latest commit of the given owner/module repository.
func (h *Handler) GetFileContent(ctx context.Context, req *connect.Request[registryv1.GetFileContentRequest]) (*connect.Response[registryv1.GetFileContentResponse], error) {
	content, size, err := h.treeStorage.GetFileContent(ctx, req.Msg.Owner, req.Msg.Module, req.Msg.Path)
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
