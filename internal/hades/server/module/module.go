package module

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Server struct {
	registryv1.ModuleServiceHandler

	logger                  *log.LoggerWrapper
	moduleDBStorage         *moduledb.ModuleStorage
	commitDBStorage         *commitdb.CommitStorage
	gitalyRepositoryService *repository.RepositoryService
	gitalyOperationService  *operation.OperationService
	authorization           *authorization.Server
	blobStorage             *blob.BlobService
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

func (s *Server) CreateModuleByName(ctx context.Context, in *connect.Request[registrypbv1.CreateModuleByNameRequest]) (*connect.Response[registrypbv1.CreateModuleByNameResponse], error) {

	in.Msg.Name = strings.ToLower(in.Msg.Name)

	user, ok := ctx.Value("user").(*registrypbv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	moduleFullName := user.Username + "/" + in.Msg.Name

	can, err := s.authorization.Can(ctx, &constants.Policy{
		Subject: user.Username,
		Object:  string(constants.REPOSITORY),
		Action:  string(constants.CREATE),
		Domain:  moduleFullName,
	})
	if err != nil {
		return nil, err
	}

	if !can.Allowed {
		return nil, pkgerr.New("User is not allowed to create this repo", pkgerr.PermissionDenied)
	}

	model := &registrypbv1.Module{}

	model.OwnerId = user.Id
	model.Name = moduleFullName

	module, err := s.moduleDBStorage.Create(
		ctx,
		moduleFullName,
		user.Id,
		registrypbv1.ModuleVisibility(in.Msg.Visibility),
		registrypbv1.ModuleState(registrypbv1.EState_E_STATE_ACTIVE),
		in.Msg.Description,
		"",
		"",
		in.Msg.DefaultBranch,
	)
	if err != nil {
		return nil, pkgerr.FromPgx(err)
	}

	// TODO
	// It can be ommited because we have username/* owner role
	roles := []*constants.Role{
		{
			User:   user.Username,
			Role:   string(constants.OWNER),
			Domain: moduleFullName,
		},
	}
	policies := []*constants.Policy{
		{
			Subject: user.Username,
			Domain:  moduleFullName,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.PUSH),
		},
		{
			Subject: user.Username,
			Domain:  moduleFullName,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.READ),
		},
	}
	err = s.authorization.AddPoliciesRolse(ctx, policies, roles)
	if err != nil {
		return nil, err
	}

	// TODO should create a commit in gitaly as well
	// Init commit
	err = s.gitalyRepositoryService.CreateRepository(ctx, model)
	if err != nil {
		return nil, err
	}

	content := &registrypbv1.UploadRequestContent{
		// ModuleRef: &models.ModuleRef{
		// 	Owner:  module.Owner.Username,
		// 	Module: module.Name,
		// },
		Files: []*registrypbv1.File{
			{
				Path:    "README.md",
				Content: []byte(""),
			},
		},
	}

	paths := []string{}
	digestValue, err := shake256.DigestFiles(content.Files)

	hash, err := s.gitalyOperationService.UserCommitFiles(ctx, module, content.Files, user, paths, digestValue.String())
	if err != nil {
		return nil, err
	}

	err = s.commitDBStorage.Create(
		ctx,
		uuid.New(),
		hash,
		user.Id,
		module.Id,
		registrypbv1.DigestType_DIGEST_TYPE_B5,
		"",
		user.Id,
		"",
	)
	if err != nil {
		return nil, err
	}

	return &connect.Response[registrypbv1.CreateModuleByNameResponse]{
		Msg: &registrypbv1.CreateModuleByNameResponse{
			Module: module,
		},
	}, nil
}
