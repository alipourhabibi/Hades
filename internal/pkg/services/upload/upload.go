package upload

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
	"github.com/alipourhabibi/Hades/internal/pkg/services/authorization"
	dbcommit "github.com/alipourhabibi/Hades/internal/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/commit"
	"github.com/alipourhabibi/Hades/internal/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/paths"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Service struct {
	commitService        *commit.CommitService
	dbcommitService      *dbcommit.CommitStorage
	operationService     *operation.OperationService
	dbmodule             *module.ModuleStorage
	authorizationService *authorization.Service
	blobStorage          *blob.BlobService

	logger *log.LoggerWrapper
}

func NewService(l *log.LoggerWrapper, commitService *commit.CommitService, operationService *operation.OperationService, m *module.ModuleStorage, dbcommit *dbcommit.CommitStorage, b *blob.BlobService, authorizationService *authorization.Service) (*Service, error) {
	return &Service{
		logger:               l,
		commitService:        commitService,
		operationService:     operationService,
		dbmodule:             m,
		dbcommitService:      dbcommit,
		authorizationService: authorizationService,
		blobStorage:          b,
	}, nil
}

// TODO refactor it
func (s *Service) Upload(ctx context.Context, req *models.UploadRequest) ([]*models.Commit, error) {
	// TODO check req.DepCommitIds

	user, ok := ctx.Value("user").(*models.User)
	if !ok {
		return nil, pkgerr.New("Internal", pkgerr.Internal)
	}

	commits := []*models.Commit{}

	for _, content := range req.Contents {

		moduleFullName := content.ModuleRef.Owner + "/" + content.ModuleRef.Module
		pol := &models.Policy{
			Subject: user.Username,
			Object:  string(models.REPOSITORY),
			Action:  string(models.PUSH),
			Domain:  moduleFullName,
		}
		can, err := s.authorizationService.Can(ctx, pol)
		if err != nil {
			return nil, pkgerr.FromCasbin(err)
		}
		if !can.Allowed {
			return nil, pkgerr.New("Permission Denied getting module "+moduleFullName, pkgerr.PermissionDenied)
		}

		module, err := s.dbmodule.GetModulesByRefs(ctx, content.ModuleRef)
		if err != nil {
			return nil, err
		}

		if len(module) == 0 {
			return nil, pkgerr.New("Module Not Found", pkgerr.NotFound)
		}

		// get the last commit for this module
		var emptyCommit bool
		moduleCommit, err := s.dbcommitService.GetCommitByOwnerModule(ctx, []*models.ModuleRef{content.ModuleRef})
		if err != nil {
			pkgErr := pkgerr.FromGorm(err).(pkgerr.PkgError)
			if pkgErr.Code != pkgerr.NotFound {
				return nil, pkgErr
			} else {
				emptyCommit = true
			}
		}

		var files []*models.File
		var listFiles []string
		if !emptyCommit {

			// get the blobs of the last commits
			blobs, err := s.blobStorage.ListBlobs(ctx, moduleCommit[0])
			if err != nil {
				return nil, err
			}

			uploadFiles := map[string]*models.File{}

			for _, f := range blobs[0].Files {
				uploadFiles[f.Path] = &models.File{
					Path:    f.Path,
					Content: f.Content,
				}
			}

			for _, f := range content.Files {
				uploadFiles[f.Path] = &models.File{
					Path:    f.Path,
					Content: f.Content,
				}
			}

			files = make([]*models.File, 0, len(uploadFiles))
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
			files = []*models.File{}
		}

		digest, err := shake256.DigestFiles(files)
		if err != nil {
			return nil, err
		}

		dig, _ := strings.CutPrefix(digest.String(), "shake256:")
		commit, err := s.dbcommitService.GetCommitByQuery(ctx, map[string]any{
			"digest_value": dig,
		})
		if err != nil {
			err := pkgerr.FromGorm(err).(pkgerr.PkgError)
			if err.Code != pkgerr.NotFound {
				return nil, err
			}
		}

		// It found commit so we add it and continue
		if err == nil && commit != nil {
			commitDigestBttes, err := hex.DecodeString(commit.DigestValue)
			if err != nil {
				return nil, err
			}
			commit.DigestValue = string(commitDigestBttes)
			commits = append(commits, commit)
			continue
		}

		files = paths.GetPath(files)
		commitId, err := s.operationService.UserCommitFiles(ctx, module[0], files, user, listFiles, dig)
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

		commit = &models.Commit{
			ID:              id,
			CommitHash:      commitId,
			DigestType:      models.DigestType_B5,
			DigestValue:     digestStr,
			OwnerID:         user.ID,
			ModuleID:        module[0].ID,
			CreatedByUserID: user.ID,
		}
		err = s.dbcommitService.Create(ctx, commit)
		if err != nil {
			return nil, pkgerr.FromGorm(err)
		}

		// the return value should be the non hex encoded digest
		commit.DigestValue = string(digest.Value())

		commits = append(commits, commit)
		// TODO make a revert mechanism if one of them failed
	}

	return commits, nil
}
