package bufdownload

import (
	"context"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/internal/pkg/services/bufdownload"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

type Server struct {
	modulev1connect.DownloadServiceHandler

	service *bufdownload.Service
	logger  *log.LoggerWrapper
}

func NewServer(l *log.LoggerWrapper, service *bufdownload.Service) *Server {
	return &Server{
		logger:  l,
		service: service,
	}
}

func (s *Server) Download(ctx context.Context, req *connect.Request[modulev1.DownloadRequest]) (*connect.Response[modulev1.DownloadResponse], error) {
	refs := []*models.ModuleRef{}
	for _, ref := range req.Msg.Values {
		id, err := uuid.Parse(ref.GetResourceRef().GetId())
		if err != nil {
			return nil, err
		}
		refs = append(refs, &models.ModuleRef{
			Id:     id.String(),
			Owner:  ref.GetResourceRef().GetName().GetOwner(),
			Module: ref.GetResourceRef().GetName().GetModule(),
		})
	}

	downloaded, err := s.service.Downalod(ctx, refs)
	if err != nil {
		return nil, err
	}

	contents := []*modulev1.DownloadResponse_Content{}
	for _, d := range downloaded {
		contents = append(contents, models.ToContentPB(d))
	}

	return &connect.Response[modulev1.DownloadResponse]{
		Msg: &modulev1.DownloadResponse{
			Contents: contents,
		},
	}, nil
}
