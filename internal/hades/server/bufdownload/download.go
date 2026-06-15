// Package bufdownload implements the buf.build DownloadService protocol adapter.
// It translates buf.build wire types to internal types and delegates all
// business logic to the download.Handler.
package bufdownload

import (
	"context"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/buf/dto"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/download"
	"github.com/alipourhabibi/Hades/utils/log"
)

// downloadProvider is the interface the Server delegates to.
// download.Handler satisfies it; tests can provide a fake.
type downloadProvider interface {
	Download(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error)
}

// Server is the buf.build protocol adapter for download.
// All business logic lives in download.Handler (own handler).
type Server struct {
	modulev1connect.DownloadServiceHandler

	handler downloadProvider
	logger  *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:  deps.Logger,
		handler: download.New(deps),
	}
}

func (s *Server) Download(ctx context.Context, req *connect.Request[modulev1.DownloadRequest]) (*connect.Response[modulev1.DownloadResponse], error) {
	refs := make([]*registryv1.ModuleRef, 0, len(req.Msg.Values))
	for _, ref := range req.Msg.Values {
		refs = append(refs, dto.FromResourceRefPB(ref.GetResourceRef()))
	}

	contents, err := s.handler.Download(ctx, refs)
	if err != nil {
		return nil, err
	}

	contentsResp := make([]*modulev1.DownloadResponse_Content, 0, len(contents))
	for _, d := range contents {
		contentsResp = append(contentsResp, dto.ToContentPB(d))
	}

	return &connect.Response[modulev1.DownloadResponse]{
		Msg: &modulev1.DownloadResponse{Contents: contentsResp},
	}, nil
}
