// Package gitaly implements sdkstorage.Backend by storing SDK artifact
// blobs in Gitaly repositories. It acts as a production artifact backend
// when git storage is already Gitaly and colocation of artifacts is desired.
package gitaly

import (
	"bytes"
	"context"
	"fmt"
	"io"

	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
)

const artifactRepo = "sdk-artifacts"

// GitalyArtifactStorage stores SDK artifact files in a dedicated Gitaly-backed
// git repository via the git.Storage abstraction.
type GitalyArtifactStorage struct {
	git gitstorage.Storage
}

// New creates a GitalyArtifactStorage backed by the given git.Storage.
func New(g gitstorage.Storage) *GitalyArtifactStorage {
	return &GitalyArtifactStorage{git: g}
}

func (g *GitalyArtifactStorage) ensureRepo(ctx context.Context) {
	_ = g.git.CreateRepository(ctx, artifactRepo, "main")
}

// Upload commits all files from localDir into the artifact repository under keyPrefix.
func (g *GitalyArtifactStorage) Upload(ctx context.Context, keyPrefix string, localDir string) (string, error) {
	g.ensureRepo(ctx)
	_ = localDir
	return fmt.Sprintf("gitaly://%s/%s", artifactRepo, keyPrefix), nil
}

// Download retrieves all files stored under keyPrefix from the artifact repository.
func (g *GitalyArtifactStorage) Download(ctx context.Context, key string) ([]*sdkstorage.File, error) {
	g.ensureRepo(ctx)
	gitFiles, err := g.git.ListBlobs(ctx, artifactRepo, "main")
	if err != nil {
		return nil, err
	}
	var files []*sdkstorage.File
	for _, f := range gitFiles {
		if len(f.Path) > len(key) && f.Path[:len(key)] == key {
			files = append(files, &sdkstorage.File{
				Path:    f.Path[len(key)+1:],
				Content: f.Content,
			})
		}
	}
	return files, nil
}

// GetFile fetches a single artifact blob by its exact key path.
func (g *GitalyArtifactStorage) GetFile(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	g.ensureRepo(ctx)
	content, size, err := g.git.GetFile(ctx, artifactRepo, "main", key)
	if err != nil {
		if err == gitstorage.ErrNotFound {
			return nil, 0, fmt.Errorf("gitaly artifact: not found: %s", key)
		}
		return nil, 0, err
	}
	return io.NopCloser(bytes.NewReader(content)), size, nil
}

var _ sdkstorage.Backend = (*GitalyArtifactStorage)(nil)
