package upload

import (
	"context"
	"encoding/hex"
	"strings"

	modulev1connect "buf.build/gen/go/bufbuild/registry/connectrpc/go/buf/registry/module/v1/modulev1connect"
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
	"github.com/alipourhabibi/Hades/utils/paths"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Server struct {
	modulev1connect.UploadServiceHandler

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
		moduleDBStorage:         deps.ModuleDB,
		commitDBStorage:         deps.CommitDB,
		gitalyRepositoryService: deps.GitalyRepositoryStorage,
		gitalyOperationService:  deps.GitalyOperationStorage,
		authorization:           deps.Authorization,
		blobStorage:             deps.GitalyBlobStorage,
	}
}

func (s *Server) Upload(ctx context.Context, req *connect.Request[modulev1.UploadRequest]) (*connect.Response[modulev1.UploadResponse], error) {

	user, ok := ctx.Value("user").(*registryv1.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	uploadRequest := &registryv1.UploadRequest{}
	for _, r := range req.Msg.Contents {
		content := &registryv1.UploadRequestContent{
			ModuleRef: &registryv1.ModuleRef{
				Id:     r.ModuleRef.GetId(),
				Owner:  r.ModuleRef.GetName().GetOwner(),
				Module: r.ModuleRef.GetName().GetModule(),
			},
			Files: make([]*registryv1.File, 0, len(r.Files)),
		}

		for _, f := range r.Files {
			content.Files = append(content.Files, &registryv1.File{
				Path:    f.Path,
				Content: f.Content,
			})
		}

		uploadRequest.Contents = append(uploadRequest.Contents, content)
	}

	commits := []*registryv1.Commit{}

	for _, content := range uploadRequest.Contents {

		moduleFullName := content.ModuleRef.Owner + "/" + content.ModuleRef.Module
		pol := &constants.Policy{
			Subject: user.Username,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.PUSH),
			Domain:  moduleFullName,
		}
		can, err := s.authorization.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}

		module, err := s.moduleDBStorage.GetModulesByRefs(ctx, content.ModuleRef)
		if err != nil {
			return nil, err
		}

		if len(module) == 0 {
			return nil, pkgerr.New("Module Not Found", pkgerr.NotFound)
		}

		// get the last commit for this module
		var emptyCommit bool
		moduleCommit, err := s.commitDBStorage.GetCommitByOwnerModule(ctx, []*registryv1.ModuleRef{content.ModuleRef})
		if err != nil {
			pkgErr := pkgerr.FromPgx(err).(pkgerr.PkgError)
			if pkgErr.Code != pkgerr.NotFound {
				return nil, pkgErr
			} else {
				emptyCommit = true
			}
		}

		var files []*registryv1.File
		var listFiles []string
		if !emptyCommit {

			// get the blobs of the last commits
			blobs, err := s.blobStorage.ListBlobs(ctx, moduleCommit[0])
			if err != nil {
				return nil, err
			}

			uploadFiles := map[string]*registryv1.File{}

			for _, f := range blobs[0].Files {
				uploadFiles[f.Path] = &registryv1.File{
					Path:    f.Path,
					Content: f.Content,
				}
			}

			for _, f := range content.Files {
				uploadFiles[f.Path] = &registryv1.File{
					Path:    f.Path,
					Content: f.Content,
				}
			}

			files = make([]*registryv1.File, 0, len(uploadFiles))
			for _, f := range uploadFiles {
				files = append(files, f)
			}

			// paths are only ones that exists in blob
			listFiles = make([]string, 0, len(files))
			for _, f := range blobs[0].Files {
				listFiles = append(listFiles, f.Path)
			}
		} else {
			listFiles = []string{}
			files = []*registryv1.File{}
		}

		digest, err := shake256.DigestFiles(files)
		if err != nil {
			return nil, err
		}

		dig, _ := strings.CutPrefix(digest.String(), "shake256:")
		commit, err := s.commitDBStorage.GetCommitByQuery(ctx, map[string]any{
			"digest_value": dig,
		})
		if err != nil {
			err := pkgerr.FromPgx(err).(pkgerr.PkgError)
			if err.Code != pkgerr.NotFound {
				return nil, err
			}
		}

		// It found commit so we add it and continue
		if err == nil && commit != nil {
			commitDigestBttes, err := hex.DecodeString(string(commit.Digest.Value))
			if err != nil {
				return nil, err
			}
			commit.Digest.Value = commitDigestBttes
			commits = append(commits, commit)
			continue
		}

		files = paths.GetPath(files)
		commitId, err := s.gitalyOperationService.UserCommitFiles(ctx, module[0], files, user, listFiles, dig)
		if err != nil {
			return nil, err
		}

		commitUUID := ""
		if len(commitId) < 32 {
			err = pkgerr.New("commitId is less than 32", pkgerr.Internal)
			return nil, err
		} else {
			commitUUID = commitId[:32]
		}

		id, err := uuid.Parse(commitUUID)
		if err != nil {
			err = pkgerr.New("Can not parse commitUUID", pkgerr.Internal)
			return nil, err
		}

		digestStr, _ := strings.CutPrefix(string(digest.String()), "shake256:")

		err = s.commitDBStorage.Create(
			ctx,
			id,
			commitId,
			user.Id,
			module[0].Id,
			registryv1.DigestType_DIGEST_TYPE_B5,
			digestStr,
			user.Id,
			"",
		)
		if err != nil {
			return nil, pkgerr.FromPgx(err)
		}

		commit = &registryv1.Commit{
			Id:         id.String(),
			CommitHash: commitId,
			OwnerId:    user.Id,
			ModuleId:   module[0].Id,
			Digest: &registryv1.Digest{
				Value: digest.Value(),
				Type:  registryv1.DigestType_DIGEST_TYPE_B5,
			},
		}

		// the return value should be the non hex encoded digest
		commits = append(commits, commit)
		// TODO make a revert mechanism if one of them failed
	}

	responseCommits := []*modulev1.Commit{}
	for _, v := range commits {
		responseCommits = append(responseCommits, dto.ToCommitPB(v))
	}

	return &connect.Response[modulev1.UploadResponse]{
		Msg: &modulev1.UploadResponse{
			Commits: responseCommits,
		},
	}, nil

}
