package gogit

import (
	"context"
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-billy/v5/osfs"
)

func (g *GoGitStorage) CreateRepository(ctx context.Context, repoPath, defaultBranch string) error {
	path := g.repoPath(repoPath)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("gogit: mkdir %s: %w", path, err)
	}
	fs := osfs.New(path)
	s := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	if _, err := gogit.InitWithOptions(s, nil, gogit.InitOptions{
		DefaultBranch: plumbing.NewBranchReferenceName(defaultBranch),
	}); err != nil && err != gogit.ErrRepositoryAlreadyExists {
		return fmt.Errorf("gogit: init %s: %w", path, err)
	}
	return nil
}

func (g *GoGitStorage) DeleteRepository(_ context.Context, repoPath string) error {
	return os.RemoveAll(g.repoPath(repoPath))
}
