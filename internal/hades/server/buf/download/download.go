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
	"github.com/alipourhabibi/Hades/internal/hades/server/content"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
)

type downloadProvider interface {
	Download(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error)
}

// resourceResolver resolves a resource UUID to its type.
type resourceResolver interface {
	ResolveType(ctx context.Context, id string) (resource.ResourceType, error)
}

type Server struct {
	modulev1connect.UnimplementedDownloadServiceHandler

	handler  downloadProvider
	resolver resourceResolver
	logger   *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:   deps.Logger,
		handler:  content.NewHandler(deps),
		resolver: deps.ResourceDB,
	}
}

func (s *Server) Download(ctx context.Context, req *connect.Request[modulev1.DownloadRequest]) (*connect.Response[modulev1.DownloadResponse], error) {
	var commitIDs []string
	var moduleRefs []*registryv1.ModuleRef

	for _, v := range req.Msg.Values {
		r := v.GetResourceRef()
		if r.GetId() != "" {
			rt, err := s.resolver.ResolveType(ctx, r.GetId())
			if err != nil {
				// Translated rather than returned raw. The error interceptor
				// flattens anything that is not already a connect error to
				// Internal, so an unregistered id surfaced as a 500 and a
				// database failure surfaced as whatever the driver produced.
				return nil, connerr.FromDB(err)
			}
			switch rt {
			case resource.ResourceTypeCommit:
				commitIDs = append(commitIDs, r.GetId())
			case resource.ResourceTypeModule:
				moduleRefs = append(moduleRefs, &registryv1.ModuleRef{Id: r.GetId()})
			case resource.ResourceTypeLabel:
				return nil, connerr.Unimplemented("label refs are not yet supported in Download")
			default:
				return nil, connerr.Unimplemented("unknown resource type for id: " + r.GetId())
			}
		} else {
			moduleRefs = append(moduleRefs, &registryv1.ModuleRef{
				Owner:  r.GetName().GetOwner(),
				Module: r.GetName().GetModule(),
			})
		}
	}

	contents, err := s.handler.Download(ctx, commitIDs, moduleRefs)
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
