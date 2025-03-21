package bufgraph

import (
	"context"
	"encoding/hex"

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
	modulev1connect.GraphServiceHandler

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

func (s *Server) GetGraph(ctx context.Context, req *connect.Request[modulev1.GetGraphRequest]) (*connect.Response[modulev1.GetGraphResponse], error) {

	user, ok := ctx.Value("user").(*registryv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}
	// TODO we may need to use resourceRefs model instead of moduleRef because of label and ref
	refs := make([]*registryv1.ModuleRef, 0, len(req.Msg.ResourceRefs))
	for _, r := range req.Msg.ResourceRefs {
		id, err := uuid.Parse(r.GetId())
		if err != nil {
			return nil, err
		}

		ref := &registryv1.ModuleRef{
			Id:     id.String(),
			Owner:  r.GetName().GetOwner(),
			Module: r.GetName().GetModule(),
		}
		refs = append(refs, ref)
	}

	modules, err := s.moduleDBStorage.GetModulesByRefs(ctx, refs...)
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
		can, err := s.authorization.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}
	}

	// TODO this should be changed and get the graph
	commits, err := s.commitDBStorage.GetCommitByOwnerModule(ctx, refs)

	if err != nil {
		return nil, err
	}

	moduleV1Commits := make([]*modulev1.Commit, 0, len(commits))
	for _, c := range commits {
		mv1commit := dto.ToCommitPB(c)
		// TODO better way?
		mv1commit.Digest.Value, _ = hex.DecodeString(string(mv1commit.Digest.Value))
		// dig, err := shake256.NewDigestForContent(bytes.NewReader(mv1commit.Digest.Value))
		// if err != nil {
		// 	return nil, err
		// }
		// mv1commit.Digest.Value = dig.Value()
		moduleV1Commits = append(moduleV1Commits, mv1commit)
	}

	return &connect.Response[modulev1.GetGraphResponse]{
		Msg: &modulev1.GetGraphResponse{
			Graph: &modulev1.Graph{
				Commits: moduleV1Commits,
			},
		},
	}, nil
}
