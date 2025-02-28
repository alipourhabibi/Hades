package upload

import (
	"context"

	modulev1connect "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/internal/pkg/services/upload"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	modulev1connect.UploadServiceHandler

	service *upload.Service

	logger *log.LoggerWrapper
}

func NewServer(l *log.LoggerWrapper, service *upload.Service) *Server {
	return &Server{
		logger:  l,
		service: service,
	}
}

func (s *Server) Upload(ctx context.Context, req *connect.Request[modulev1.UploadRequest]) (*connect.Response[modulev1.UploadResponse], error) {

	uploadRequest := &models.UploadRequest{}
	for _, r := range req.Msg.Contents {
		content := &models.UploadRequest_Content{
			ModuleRef: &models.ModuleRef{
				Id:     r.ModuleRef.GetId(),
				Owner:  r.ModuleRef.GetName().GetOwner(),
				Module: r.ModuleRef.GetName().GetModule(),
			},
			Files: make([]*models.File, 0, len(r.Files)),
		}

		for _, f := range r.Files {
			content.Files = append(content.Files, &models.File{
				Path:    f.Path,
				Content: f.Content,
			})
		}

		uploadRequest.Contents = append(uploadRequest.Contents, content)
	}

	commits, err := s.service.Upload(ctx, uploadRequest)
	if err != nil {
		return nil, err
	}

	responseCommits := []*modulev1.Commit{}
	for _, v := range commits {
		responseCommits = append(responseCommits, models.ToCommitPB(v))
	}

	return &connect.Response[modulev1.UploadResponse]{
		Msg: &modulev1.UploadResponse{
			Commits: responseCommits,
		},
	}, nil

}
