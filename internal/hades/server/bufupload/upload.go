// Package bufupload implements the buf.build UploadService protocol adapter.
// It converts buf.build wire types to internal proto types, delegates all
// business logic to the upload.Handler, and converts results back.
package bufupload

import (
	"context"

	modulev1connect "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/buf/dto"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/upload"
	"github.com/alipourhabibi/Hades/utils/log"
)

// uploadProvider is the interface the Server delegates to.
// upload.Handler satisfies it; tests can provide a fake.
type uploadProvider interface {
	Upload(ctx context.Context, contents []*registryv1.UploadRequestContent) ([]*registryv1.Commit, error)
}

// Server is the buf.build protocol adapter for upload.
// It converts buf.build wire types to own proto types, delegates all business
// logic to upload.Handler, then converts the result back.
// The own CLI will call upload.Handler.Upload directly.
type Server struct {
	modulev1connect.UploadServiceHandler

	handler uploadProvider
	logger  *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:  deps.Logger,
		handler: upload.NewHandler(deps),
	}
}

func (s *Server) Upload(ctx context.Context, req *connect.Request[modulev1.UploadRequest]) (*connect.Response[modulev1.UploadResponse], error) {
	contents := make([]*registryv1.UploadRequestContent, 0, len(req.Msg.Contents))
	for _, r := range req.Msg.Contents {
		contents = append(contents, dto.FromUploadContentPB(r))
	}

	commits, err := s.handler.Upload(ctx, contents)
	if err != nil {
		return nil, err
	}

	responseCommits := make([]*modulev1.Commit, 0, len(commits))
	for _, v := range commits {
		responseCommits = append(responseCommits, dto.ToCommitPB(v))
	}

	return &connect.Response[modulev1.UploadResponse]{
		Msg: &modulev1.UploadResponse{Commits: responseCommits},
	}, nil
}
