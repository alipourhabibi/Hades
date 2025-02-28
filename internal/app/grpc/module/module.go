package module

import (
	"context"

	"connectrpc.com/connect"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/internal/pkg/services/module"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	registryv1.ModuleServiceHandler

	logger  *log.LoggerWrapper
	service *module.Service
}

func NewServer(l *log.LoggerWrapper, service *module.Service) *Server {
	return &Server{
		logger:  l,
		service: service,
	}
}

func (s *Server) CreateModuleByName(ctx context.Context, in *connect.Request[registrypbv1.CreateModuleByNameRequest]) (*connect.Response[registrypbv1.CreateModuleByNameResponse], error) {

	module, err := s.service.CreateByNameModule(ctx, &models.Module{
		Name:             in.Msg.Name,
		Visibility:       models.ModuleVisibility(in.Msg.Visibility),
		Description:      in.Msg.Description,
		DefaultLabelName: in.Msg.DefaultBranch,
	})
	if err != nil {
		return nil, err
	}

	return &connect.Response[registrypbv1.CreateModuleByNameResponse]{
		Msg: &registrypbv1.CreateModuleByNameResponse{
			Module: models.ToModulePB(module),
		},
	}, nil
}
