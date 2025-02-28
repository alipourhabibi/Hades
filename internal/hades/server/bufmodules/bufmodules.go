package bufmodule

import (
	"context"

	moduleConnV1 "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/buf/dto"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Server struct {
	moduleConnV1.ModuleServiceHandler

	moduleDBStorage         *moduledb.ModuleStorage
	commitDBStorage         *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorization           *authorization.Server
	blobStorage             *blob.BlobService
	logger                  *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:                  deps.Logger,
		commitDBStorage:         deps.CommitDB,
		moduleDBStorage:         deps.ModuleDB,
		gitalyRepositoryService: deps.GitalyRepositoryStorage,
		gitalyOperationService:  deps.GitalyOperationStorage,
		authorization:           deps.Authorization,
		blobStorage:             deps.GitalyBlobStorage,
	}
}

func (m *Server) GetModules(ctx context.Context, req *connect.Request[modulev1.GetModulesRequest]) (*connect.Response[modulev1.GetModulesResponse], error) {

	user, ok := ctx.Value("user").(*registryv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	in := []*registryv1.ModuleRef{}
	for _, v := range req.Msg.ModuleRefs {
		if v.GetId() != "" {
			in = append(in, &registryv1.ModuleRef{
				Id: v.GetId(),
			})
		} else {
			in = append(in, &registryv1.ModuleRef{
				Owner:  v.GetName().GetOwner(),
				Module: v.GetName().GetModule(),
			})
		}
	}

	modules, err := m.moduleDBStorage.GetModulesByRefs(ctx, in...)
	if err != nil {
		return nil, err
	}

	for _, module := range modules {
		// TODO check the state of the module
		// if module.Visibility != models.ModuleVisibility_MODULE_VISIBILITY_PRIVATE {
		// 	continue
		// }
		moduleFullName := module.Name
		pol := &constants.Policy{
			Subject: user.Username,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.READ),
			Domain:  moduleFullName,
		}
		can, err := m.authorization.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}
	}

	responseModules := []*modulev1.Module{}
	for _, m := range modules {
		responseModules = append(responseModules, dto.ToBufModulePB(m))
	}

	return &connect.Response[modulev1.GetModulesResponse]{
		Msg: &modulev1.GetModulesResponse{
			Modules: responseModules,
		},
	}, nil

}
