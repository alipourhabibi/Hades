// Package bufmodules implements the buf.build ModuleService protocol adapter.
// It translates buf.build wire types to internal types and delegates all
// business logic to the module.Server.
package bufmodules

import (
	"context"

	moduleConnV1 "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/buf/dto"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/module"
	"github.com/alipourhabibi/Hades/utils/log"
)

// modulesProvider is the interface the Server delegates to.
// module.Server satisfies it; tests can provide a fake.
type modulesProvider interface {
	GetModules(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Module, error)
}

// Server is the buf.build protocol adapter for module queries.
// All business logic lives in module.Server (own handler).
type Server struct {
	moduleConnV1.ModuleServiceHandler

	handler modulesProvider
	logger  *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:  deps.Logger,
		handler: module.NewServer(deps),
	}
}

func (s *Server) GetModules(ctx context.Context, req *connect.Request[modulev1.GetModulesRequest]) (*connect.Response[modulev1.GetModulesResponse], error) {
	refs := make([]*registryv1.ModuleRef, 0, len(req.Msg.ModuleRefs))
	for _, v := range req.Msg.ModuleRefs {
		refs = append(refs, dto.FromModuleRefPB(v))
	}

	modules, err := s.handler.GetModules(ctx, refs)
	if err != nil {
		return nil, err
	}

	responseModules := make([]*modulev1.Module, 0, len(modules))
	for _, mod := range modules {
		responseModules = append(responseModules, dto.ToBufModulePB(mod))
	}

	return &connect.Response[modulev1.GetModulesResponse]{
		Msg: &modulev1.GetModulesResponse{Modules: responseModules},
	}, nil
}
