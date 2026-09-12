// Package gitaly implements sdkstorage.Backend by storing SDK artifact
// blobs in Gitaly repositories. It acts as a production artifact backend
// when git storage is already Gitaly and colocation of artifacts is desired.
package gitaly

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/utils/paths"
)

const (
	artifactRepo   = "sdk-artifacts"
	artifactBranch = "main"
)

// GitalyArtifactStorage stores SDK artifact files in a dedicated Gitaly-backed
// git repository via the git.Storage abstraction.
type GitalyArtifactStorage struct {
	git gitstorage.Storage
}

// New creates a GitalyArtifactStorage backed by the given git.Storage.
func New(g gitstorage.Storage) *GitalyArtifactStorage {
	return &GitalyArtifactStorage{git: g}
}

// ensureRepo makes sure the one shared artifact repository exists.
//
// git.Storage.CreateRepository is documented idempotent, which is what makes
// this the whole implementation. It was not idempotent on the Gitaly backend,
// so this returned AlreadyExists from the second SDK job onwards and every job
// after the first failed permanently: nothing deletes the artifact repository,
// so there was no recovery short of intervening in Gitaly by hand.
func (g *GitalyArtifactStorage) ensureRepo(ctx context.Context) error {
	return g.git.CreateRepository(ctx, artifactRepo, artifactBranch)
}

// Upload commits every file under localDir into the artifact repository at
// keyPrefix and returns the location URI.
//
// It used to discard localDir and return a synthesised URI, so every SDK job
// was marked succeeded with a plausible-looking location and nothing was
// stored. The failure was invisible until someone tried to download a
// generated SDK.
func (g *GitalyArtifactStorage) Upload(ctx context.Context, keyPrefix string, localDir string) (string, error) {
	if err := paths.ValidatePath(keyPrefix); err != nil {
		return "", fmt.Errorf("gitaly artifact: key prefix %q: %w", keyPrefix, err)
	}
	if err := g.ensureRepo(ctx); err != nil {
		return "", fmt.Errorf("gitaly artifact: ensure repository: %w", err)
	}

	var files []*gitstorage.File
	err := filepath.WalkDir(localDir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := paths.ValidatePath(rel); err != nil {
			return fmt.Errorf("gitaly artifact: refusing %q: %w", rel, err)
		}
		// #nosec G304,G122 -- p comes from WalkDir over the temporary directory this process created; the relative path is validated above.
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files = append(files, &gitstorage.File{
			Path:    path.Join(keyPrefix, rel),
			Content: content,
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("gitaly artifact: %s produced no files to store", localDir)
	}

	// The existing paths under this prefix are replaced wholesale, so a
	// regenerated SDK that no longer emits a file removes it.
	existing, err := g.ListFiles(ctx, keyPrefix)
	if err != nil && !errors.Is(err, gitstorage.ErrNotFound) {
		return "", err
	}
	existingKeys := make([]string, 0, len(existing))
	for _, rel := range existing {
		existingKeys = append(existingKeys, path.Join(keyPrefix, rel))
	}

	head, err := g.head(ctx)
	if err != nil {
		return "", err
	}

	if _, err := g.git.PutFiles(ctx, gitstorage.PutFilesRequest{
		RepoPath:      artifactRepo,
		Branch:        artifactBranch,
		Files:         files,
		ExistingPaths: existingKeys,
		AuthorName:    "hades",
		AuthorEmail:   "hades@localhost",
		Message:       "sdk artifacts: " + keyPrefix,
		ExpectedHead:  head,
	}); err != nil {
		return "", fmt.Errorf("gitaly artifact: commit %s: %w", keyPrefix, err)
	}

	return fmt.Sprintf("gitaly://%s/%s", artifactRepo, keyPrefix), nil
}

// head returns the artifact branch head, or "" when the branch does not exist.
func (g *GitalyArtifactStorage) head(ctx context.Context) (string, error) {
	commits, err := g.git.ListCommits(ctx, artifactRepo, artifactBranch, 1)
	if err != nil {
		if errors.Is(err, gitstorage.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("gitaly artifact: read branch head: %w", err)
	}
	if len(commits) == 0 {
		return "", nil
	}
	return commits[0].SHA, nil
}

// underPrefix reports whether p is inside keyPrefix, and returns the remainder.
//
// The comparison is on a path boundary. A plain string prefix test, which is
// what this did, matched "abc-other/x" for the key "abc".
func underPrefix(p, keyPrefix string) (string, bool) {
	if keyPrefix == "" {
		return p, true
	}
	if !strings.HasPrefix(p, keyPrefix+"/") {
		return "", false
	}
	return p[len(keyPrefix)+1:], true
}

// Download retrieves all files stored under keyPrefix from the artifact repository.
func (g *GitalyArtifactStorage) Download(ctx context.Context, key string) ([]*sdkstorage.File, error) {
	gitFiles, err := g.git.ListBlobs(ctx, artifactRepo, artifactBranch)
	if err != nil {
		return nil, err
	}
	var files []*sdkstorage.File
	for _, f := range gitFiles {
		if rel, ok := underPrefix(f.Path, key); ok {
			files = append(files, &sdkstorage.File{Path: rel, Content: f.Content})
		}
	}
	return files, nil
}

// ListFiles returns the relative paths of every blob under keyPrefix.
//
// The underlying ListBlobs reads content as well, so this does not reduce work
// against this backend; it exists so the interface has one streaming-shaped
// contract across all backends.
func (g *GitalyArtifactStorage) ListFiles(ctx context.Context, keyPrefix string) ([]string, error) {
	gitFiles, err := g.git.ListBlobs(ctx, artifactRepo, artifactBranch)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range gitFiles {
		if rel, ok := underPrefix(f.Path, keyPrefix); ok {
			out = append(out, rel)
		}
	}
	return out, nil
}

// GetFile fetches a single artifact blob by its exact key path.
func (g *GitalyArtifactStorage) GetFile(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	content, size, err := g.git.GetFile(ctx, artifactRepo, artifactBranch, key)
	if err != nil {
		// The interface sentinel, so the Go module proxy classifies a missing
		// artifact the same way on every backend.
		if errors.Is(err, gitstorage.ErrNotFound) {
			return nil, 0, fmt.Errorf("gitaly artifact %s: %w", key, sdkstorage.ErrNotFound)
		}
		return nil, 0, err
	}
	return io.NopCloser(bytes.NewReader(content)), size, nil
}

var _ sdkstorage.Backend = (*GitalyArtifactStorage)(nil)
