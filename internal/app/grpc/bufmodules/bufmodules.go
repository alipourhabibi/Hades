package bufmodule

import (
	"context"

	moduleConnV1 "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/internal/pkg/services/module"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	moduleConnV1.ModuleServiceHandler

	service *module.Service
	logger  *log.LoggerWrapper
}

func NewServer(l *log.LoggerWrapper, service *module.Service) *Server {
	return &Server{
		logger:  l,
		service: service,
	}
}

func (m *Server) GetModules(ctx context.Context, req *connect.Request[modulev1.GetModulesRequest]) (*connect.Response[modulev1.GetModulesResponse], error) {
	in := []*models.ModuleRef{}
	for _, v := range req.Msg.ModuleRefs {
		if v.GetId() != "" {
			in = append(in, &models.ModuleRef{
				Id: v.GetId(),
			})
		} else {
			in = append(in, &models.ModuleRef{
				Owner:  v.GetName().GetOwner(),
				Module: v.GetName().GetModule(),
			})
		}
	}

	modules, err := m.service.GetModules(ctx, in)
	if err != nil {
		return nil, err
	}

	responseModules := []*modulev1.Module{}
	for _, m := range modules {
		responseModules = append(responseModules, models.ToBufModulePB(m))
	}

	return &connect.Response[modulev1.GetModulesResponse]{
		Msg: &modulev1.GetModulesResponse{
			Modules: responseModules,
		},
	}, nil

}
