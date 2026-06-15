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
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/graph"
	"github.com/alipourhabibi/Hades/utils/log"
)

// graphProvider is the interface the Server delegates to.
// graph.Handler satisfies it; tests can provide a fake.
type graphProvider interface {
	GetGraph(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
}

// Server is the buf.build protocol adapter for the graph query.
// All business logic lives in graph.Handler (own handler).
type Server struct {
	modulev1connect.GraphServiceHandler

	handler graphProvider
	logger  *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:  deps.Logger,
		handler: graph.New(deps),
	}
}

func (s *Server) GetGraph(ctx context.Context, req *connect.Request[modulev1.GetGraphRequest]) (*connect.Response[modulev1.GetGraphResponse], error) {
	refs := make([]*registryv1.ModuleRef, 0, len(req.Msg.ResourceRefs))
	for _, r := range req.Msg.ResourceRefs {
		refs = append(refs, dto.FromResourceRefPB(r))
	}

	result, err := s.handler.GetGraph(ctx, refs)
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
