// Package bufgraph implements the buf.build GraphService protocol adapter.
// It translates buf.build wire types to internal types and delegates all
// business logic to the graph.Handler.
package bufgraph

import (
	"context"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/buf/dto"
	commit "github.com/alipourhabibi/Hades/internal/hades/server/commit"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// graphProvider is the interface the Server delegates to.
// graph.Handler satisfies it; tests can provide a fake.
type graphProvider interface {
	GetGraph(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
}

// resourceResolver resolves a resource UUID to its type.
type resourceResolver interface {
	ResolveType(ctx context.Context, id string) (resource.ResourceType, error)
}

// Server is the buf.build protocol adapter for the graph query.
// All business logic lives in graph.Handler (own handler).
type Server struct {
	modulev1connect.GraphServiceHandler

	handler  graphProvider
	resolver resourceResolver
	logger   *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:   deps.Logger,
		handler:  commit.NewHandler(deps),
		resolver: deps.ResourceDB,
	}
}

func (s *Server) GetGraph(ctx context.Context, req *connect.Request[modulev1.GetGraphRequest]) (*connect.Response[modulev1.GetGraphResponse], error) {
	var commitIDs []string
	var moduleRefs []*registryv1.ModuleRef

	for _, r := range req.Msg.ResourceRefs {
		if r.GetId() != "" {
			rt, err := s.resolver.ResolveType(ctx, r.GetId())
			if err != nil {
				return nil, err
			}
			switch rt {
			case resource.ResourceTypeCommit:
				commitIDs = append(commitIDs, r.GetId())
			case resource.ResourceTypeModule:
				moduleRefs = append(moduleRefs, &registryv1.ModuleRef{Id: r.GetId()})
			case resource.ResourceTypeLabel:
				return nil, connErr.Unimplemented("label refs are not yet supported in GetGraph")
			default:
				return nil, connErr.Unimplemented("unknown resource type for id: " + r.GetId())
			}
		} else {
			moduleRefs = append(moduleRefs, &registryv1.ModuleRef{
				Owner:  r.GetName().GetOwner(),
				Module: r.GetName().GetModule(),
			})
		}
	}

	result, err := s.handler.GetGraph(ctx, commitIDs, moduleRefs)
	if err != nil {
		return nil, err
	}

	moduleV1Commits := make([]*modulev1.Commit, 0, len(result))
	for _, c := range result {
		moduleV1Commits = append(moduleV1Commits, dto.ToCommitPB(c))
	}

	return &connect.Response[modulev1.GetGraphResponse]{
		Msg: &modulev1.GetGraphResponse{
			Graph: &modulev1.Graph{Commits: moduleV1Commits},
		},
	}, nil
}
