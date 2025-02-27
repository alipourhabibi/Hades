package upload

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/alipourhabibi/Hades/models"
	pkgerr "github.com/alipourhabibi/Hades/pkg/errors"
	"github.com/alipourhabibi/Hades/pkg/services/sdk"
	dbcommit "github.com/alipourhabibi/Hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/storage/db/module"
	"github.com/alipourhabibi/Hades/storage/gitaly/blob"
	"github.com/alipourhabibi/Hades/storage/gitaly/commit"
	"github.com/alipourhabibi/Hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/paths"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
)

type Service struct {
	commitService    *commit.CommitService
	dbcommitService  *dbcommit.CommitStorage
	operationService *operation.OperationService
	dbmodule         *module.ModuleStorage
	blobStorage      *blob.BlobService
	generator        *sdk.Generator

	logger *log.LoggerWrapper
}

func NewService(l *log.LoggerWrapper,
	commitService *commit.CommitService,
	operationService *operation.OperationService,
	m *module.ModuleStorage,
	dbcommit *dbcommit.CommitStorage,
	b *blob.BlobService,
	generator *sdk.Generator,
) (*Service, error) {
	return &Service{
		generator:        generator,
		logger:           l,
		commitService:    commitService,
		operationService: operationService,
		dbmodule:         m,
		dbcommitService:  dbcommit,
		blobStorage:      b,
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
	//result, err := s.uow.Do(ctx, func(ctx context.Context, tx pgx.Tx) (interface{}, error) {
	for _, content := range req.Contents {

		commit, err := s.updateContent(ctx, content, user)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)

	}

	return commits, nil
	//})

	// if err != nil {
	// 	return nil, err
	// }
	// return result.([]*models.Commit), nil

}

func (s *Service) getFiles(ctx context.Context, moduleCommit *models.Commit, content *models.UploadRequest_Content) (files []*models.File, listFiles []string, err error) {

	// get the blobs of the last commits
	blobs, err := s.blobStorage.ListBlobs(ctx, moduleCommit)
	if err != nil {
		return
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

	return
}

func (s *Service) updateContent(ctx context.Context, content *models.UploadRequest_Content, user *models.User) (*models.Commit, error) {

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
	// get files if commit is not empty
	if !emptyCommit {

		files, listFiles, err = s.getFiles(ctx, moduleCommit[0], content)
		if err != nil {
			return nil, err
		}
	}

	digest, err := shake256.DigestFiles(files)
	if err != nil {
		return nil, err
	}

	// find the shake256 of the commit in db
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
		return commit, nil
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

	return commit, nil
	// TODO make a revert mechanism if one of them failed
}
