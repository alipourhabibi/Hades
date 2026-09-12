// Package gitaly implements git.Storage by wrapping the Gitaly gRPC services.
package gitaly

import (
	"context"
	"errors"
	"strings"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GitalyStorage implements git.Storage using the Gitaly gRPC sub-services.
type GitalyStorage struct {
	repo      *RepositoryService
	op        *OperationService
	blob      *BlobService
	commitSvc *CommitService
	treeSvc   *TreeService
	diffSvc   *DiffService
	svc       *StorageService
}

// New creates a GitalyStorage from the sub-service clients in StorageService.
// The StorageService owns the shared connection, which Close releases.
func New(svc *StorageService) *GitalyStorage {
	return &GitalyStorage{
		repo:      svc.RepositoryService,
		op:        svc.OperattionService,
		blob:      svc.BlobService,
		commitSvc: svc.CommitService,
		treeSvc:   svc.TreeService,
		diffSvc:   svc.DiffService,
		svc:       svc,
	}
}

// Close releases the shared Gitaly connection.
func (g *GitalyStorage) Close() error {
	return g.svc.Close()
}

func splitPath(repoPath string) (owner, module string) {
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return repoPath, repoPath
}

func (g *GitalyStorage) CreateRepository(ctx context.Context, repoPath, defaultBranch string) error {
	// The same guard gogit applies. repoPath becomes Gitaly's RelativePath, so
	// an unvalidated module name escapes the storage layout here too.
	if err := git.ValidateRepoPath(repoPath); err != nil {
		return err
	}
	err := g.repo.CreateRepository(ctx, &registryv1.Module{
		Name:          repoPath,
		DefaultBranch: defaultBranch,
	})
	// git.Storage documents CreateRepository as idempotent, and gogit already
	// is. Gitaly reports AlreadyExists, which is the same outcome the caller
	// asked for, so it is success here rather than an error to be handled at
	// every call site.
	if grpcCode(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func (g *GitalyStorage) DeleteRepository(ctx context.Context, repoPath string) error {
	if err := git.ValidateRepoPath(repoPath); err != nil {
		return err
	}
	return g.repo.DeleteRepository(ctx, &registryv1.Module{Name: repoPath})
}

func (g *GitalyStorage) PutFiles(ctx context.Context, req git.PutFilesRequest) (string, error) {
	user := &identityv1.User{
		Id:       req.AuthorEmail,
		Username: req.AuthorName,
		Email:    req.AuthorEmail,
	}
	return g.op.UserCommitFiles(ctx, user, req)
}

func (g *GitalyStorage) RollbackCommit(ctx context.Context, repoPath, branch, currentHead, previousHead string) error {
	return g.op.RollbackCommit(ctx, &registryv1.Module{Name: repoPath, DefaultBranch: branch}, currentHead, previousHead)
}

func (g *GitalyStorage) GetFile(ctx context.Context, repoPath, ref, filePath string) ([]byte, int64, error) {
	owner, module := splitPath(repoPath)
	content, size, err := g.treeSvc.GetFileContent(ctx, owner, module, ref, filePath)
	if err != nil {
		if isNotFound(err) {
			return nil, 0, git.ErrNotFound
		}
		return nil, 0, err
	}
	return content, size, nil
}

// isNotFound reports whether err means the requested path or revision is not in
// the repository, from either our own sentinel or Gitaly's gRPC status.
func isNotFound(err error) bool {
	if errors.Is(err, ErrTreeNotFound) {
		return true
	}
	return grpcCode(err) == codes.NotFound
}

// grpcCode returns the gRPC status code carried by err, unwrapping first.
//
// status.Code does a bare type assertion, so it reports Unknown for any error
// that has been wrapped, and the sub-services here wrap everything with
// fmt.Errorf("%w"). That is why a missing file came back as Internal on the
// Gitaly backend and NotFound on gogit: the status was there, the check could
// not see it, and a client's error code depended on a server-side
// configuration choice it cannot see.
func grpcCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	var se interface{ GRPCStatus() *status.Status }
	if errors.As(err, &se) {
		return se.GRPCStatus().Code()
	}
	return codes.Unknown
}

// ListFiles lists the paths at ref.
//
// ref is honoured. It used to be accepted and ignored, so a caller asking for a
// specific commit was silently answered from the default branch.
func (g *GitalyStorage) ListFiles(ctx context.Context, repoPath, ref string) ([]string, error) {
	return g.commitSvc.ListFilesAtRef(ctx, repoPath, ref)
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
	pbEntries, err := g.treeSvc.GetTreeEntries(ctx, owner, module, ref, dir)
	if err != nil {
		if isNotFound(err) {
			return nil, git.ErrNotFound
		}
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

// ListCommits returns commits reachable from ref, newest first.
//
// It previously returned "not yet implemented" while satisfying the interface
// at compile time, so the Gitaly backend answered a supported API call with an
// error at runtime and nothing said so at wiring time.
func (g *GitalyStorage) ListCommits(ctx context.Context, repoPath, ref string, limit int) ([]*git.CommitInfo, error) {
	if limit <= 0 {
		limit = git.DefaultCommitLimit
	}
	commits, err := g.commitSvc.ListCommits(ctx, repoPath, ref, limit)
	if err != nil {
		if isNotFound(err) {
			return nil, git.ErrNotFound
		}
		return nil, err
	}
	return commits, nil
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
			TooLarge:      d.TooLarge,
		}
	}
	return diffs, nil
}

var _ git.Storage = (*GitalyStorage)(nil)
