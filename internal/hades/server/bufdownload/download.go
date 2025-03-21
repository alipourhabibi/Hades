package bufdownload

import (
	"context"

	"buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
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
	"github.com/google/uuid"
)

type Server struct {
	modulev1connect.DownloadServiceHandler

	moduleDBStorage         *moduledb.ModuleStorage
	commitDBStorage         *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorization           *authorization.Server
	blobStorage             *blob.BlobService

	logger *log.LoggerWrapper
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:                  deps.Logger,
		moduleDBStorage:         deps.ModuleDB,
		commitDBStorage:         deps.CommitDB,
		gitalyRepositoryService: deps.GitalyRepositoryStorage,
		gitalyOperationService:  deps.GitalyOperationStorage,
		authorization:           deps.Authorization,
		blobStorage:             deps.GitalyBlobStorage,
	}
}

func (s *Server) Download(ctx context.Context, req *connect.Request[modulev1.DownloadRequest]) (*connect.Response[modulev1.DownloadResponse], error) {

	user, ok := ctx.Value("user").(*registryv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	refs := []*registryv1.ModuleRef{}
	for _, ref := range req.Msg.Values {
		id, err := uuid.Parse(ref.GetResourceRef().GetId())
		if err != nil {
			return nil, err
		}
		refs = append(refs, &registryv1.ModuleRef{
			Id:     id.String(),
			Owner:  ref.GetResourceRef().GetName().GetOwner(),
			Module: ref.GetResourceRef().GetName().GetModule(),
		})
	}

	modules, err := s.moduleDBStorage.GetModulesByRefs(ctx, refs...)
	if err != nil {
		return nil, err
	}

	for _, module := range modules {
		moduleFullName := module.Name
		pol := &constants.Policy{
			Subject: user.Username,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.READ),
			Domain:  moduleFullName,
		}
		can, err := s.authorization.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}
	}

	commits, err := s.commitDBStorage.GetCommitByOwnerModule(ctx, refs)
	if err != nil {
		return nil, err
	}

	contents := []*registryv1.DownloadResponseContent{}

	for _, commit := range commits {
		content, err := s.blobStorage.ListBlobs(ctx, commit)
		if err != nil {
			return nil, err
		}
		contents = append(contents, content...)
	}

	contentsResp := []*modulev1.DownloadResponse_Content{}
	for _, d := range contents {
		contentsResp = append(contentsResp, dto.ToContentPB(d))
	}

	return &connect.Response[modulev1.DownloadResponse]{
		Msg: &modulev1.DownloadResponse{
			Contents: contentsResp,
		},
	}, nil
}
