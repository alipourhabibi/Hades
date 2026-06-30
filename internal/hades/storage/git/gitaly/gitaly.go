// Package gitaly implements git.Storage by wrapping the Gitaly gRPC services.
package gitaly

import (
	"context"
	"fmt"
	"strings"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
)

// GitalyStorage implements git.Storage using the Gitaly gRPC sub-services.
type GitalyStorage struct {
	repo      *RepositoryService
	op        *OperationService
	blob      *BlobService
	commitSvc *CommitService
	treeSvc   *TreeService
	diffSvc   *DiffService
}

// New creates a GitalyStorage from the sub-service clients in StorageService.
func New(
	repo *RepositoryService,
	op *OperationService,
	blobSvc *BlobService,
	commitSvc *CommitService,
	treeSvc *TreeService,
	diffSvc *DiffService,
) *GitalyStorage {
	return &GitalyStorage{
		repo:      repo,
		op:        op,
		blob:      blobSvc,
		commitSvc: commitSvc,
		treeSvc:   treeSvc,
		diffSvc:   diffSvc,
	}
}

func splitPath(repoPath string) (owner, module string) {
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return repoPath, repoPath
}

func (g *GitalyStorage) CreateRepository(ctx context.Context, repoPath, defaultBranch string) error {
	return g.repo.CreateRepository(ctx, &registryv1.Module{
		Name:          repoPath,
		DefaultBranch: defaultBranch,
	})
}

func (g *GitalyStorage) DeleteRepository(ctx context.Context, repoPath string) error {
	return g.repo.DeleteRepository(ctx, &registryv1.Module{Name: repoPath})
}

func (g *GitalyStorage) PutFiles(ctx context.Context, repoPath, branch string, files []*git.File, authorName, authorEmail, commitMsg string, existingPaths []string) (string, error) {
	user := &registryv1.User{
		Id:       authorEmail,
		Username: authorName,
		Email:    authorEmail,
	}
	module := &registryv1.Module{Name: repoPath, DefaultBranch: branch}
	pbFiles := make([]*registryv1.File, len(files))
	for i, f := range files {
		pbFiles[i] = &registryv1.File{Path: f.Path, Content: f.Content}
	}
	digest := ""
	if idx := strings.LastIndex(commitMsg, "digest_value:"); idx >= 0 {
		digest = strings.TrimSpace(commitMsg[idx+len("digest_value:"):])
	}
	return g.op.UserCommitFiles(ctx, module, pbFiles, user, existingPaths, digest)
}

func (g *GitalyStorage) RollbackCommit(ctx context.Context, repoPath, branch, currentHead, previousHead string) error {
	return g.op.RollbackCommit(ctx, &registryv1.Module{Name: repoPath, DefaultBranch: branch}, currentHead, previousHead)
}

func (g *GitalyStorage) GetFile(ctx context.Context, repoPath, ref, filePath string) ([]byte, int64, error) {
	owner, module := splitPath(repoPath)
	content, size, err := g.treeSvc.GetFileContent(ctx, owner, module, filePath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "file not found") {
			return nil, 0, git.ErrNotFound
		}
		return nil, 0, err
	}
	return content, size, nil
}

func (g *GitalyStorage) ListFiles(ctx context.Context, repoPath, ref string) ([]string, error) {
	owner, module := splitPath(repoPath)
	return g.commitSvc.ListFiles(ctx, &registryv1.UploadRequestContent{
		ModuleRef: &registryv1.ModuleRef{Owner: owner, Module: module},
	})
}

func (g *GitalyStorage) ListBlobs(ctx context.Context, repoPath, commitHash string) ([]*git.File, error) {
	pbCommit := &registryv1.Commit{
		CommitHash: commitHash,
		Module:     &registryv1.Module{Name: repoPath},
	}
	contents, err := g.blob.ListBlobs(ctx, pbCommit)
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 {
		return nil, nil
	}
	files := make([]*git.File, len(contents[0].Files))
	for i, f := range contents[0].Files {
		files[i] = &git.File{Path: f.Path, Content: f.Content}
	}
	return files, nil
}

func (g *GitalyStorage) StreamBlobsToDir(ctx context.Context, repoPath, commitHash, dir string) error {
	return g.blob.StreamBlobsToDir(ctx, &registryv1.Commit{
		CommitHash: commitHash,
		Module:     &registryv1.Module{Name: repoPath},
	}, dir)
}

func (g *GitalyStorage) GetTreeEntries(ctx context.Context, repoPath, ref, dir string) ([]*git.TreeEntry, error) {
	owner, module := splitPath(repoPath)
	pbEntries, err := g.treeSvc.GetTreeEntries(ctx, owner, module, dir)
	if err != nil {
		return nil, err
	}
	entries := make([]*git.TreeEntry, len(pbEntries))
	for i, e := range pbEntries {
		t := git.TreeEntryTypeFile
		if e.Type == registryv1.FileEntryType_FILE_ENTRY_TYPE_DIR {
			t = git.TreeEntryTypeDir
		}
		entries[i] = &git.TreeEntry{
			Name: e.Name,
			Path: e.Path,
			OID:  e.Oid,
			Type: t,
			Mode: e.Mode,
		}
	}
	return entries, nil
}

func (g *GitalyStorage) ListCommits(ctx context.Context, repoPath, ref string) ([]*git.CommitInfo, error) {
	return nil, fmt.Errorf("gitaly: ListCommits not yet implemented")
}

func (g *GitalyStorage) GetCommitDiff(ctx context.Context, repoPath, commitHash string) ([]*git.FileDiff, error) {
	owner, module := splitPath(repoPath)
	gitalyDiffs, err := g.diffSvc.GetCommitDiff(ctx, owner, module, commitHash)
	if err != nil {
		return nil, err
	}
	diffs := make([]*git.FileDiff, len(gitalyDiffs))
	for i, d := range gitalyDiffs {
		diffs[i] = &git.FileDiff{
			FromPath:      d.FromPath,
			ToPath:        d.ToPath,
			IsNewFile:     d.IsNewFile,
			IsDeletedFile: d.IsDeletedFile,
			IsRenamedFile: d.IsRenamedFile,
			Additions:     d.Additions,
			Deletions:     d.Deletions,
			Patch:         d.Patch,
			Binary:        d.Binary,
		}
	}
	return diffs, nil
}

var _ git.Storage = (*GitalyStorage)(nil)
