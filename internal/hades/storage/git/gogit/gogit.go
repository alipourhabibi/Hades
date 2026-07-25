// Package gogit implements git.Storage using go-git with local bare repositories.
// It is the default zero-dependency git backend for self-hosted deployments.
package gogit

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// GoGitStorage implements git.Storage using local bare repositories managed
// by go-git. Repositories are stored as bare git repos under Root.
type GoGitStorage struct {
	// Root is the directory under which all bare repositories are stored.
	Root string
}

// New creates a GoGitStorage that stores repos under root.
func New(root string) *GoGitStorage {
	if root == "" {
		root = "./data/repos"
	}
	return &GoGitStorage{Root: root}
}

func (g *GoGitStorage) repoPath(repoPath string) string {
	return filepath.Join(g.Root, repoPath)
}

func (g *GoGitStorage) openRepo(repoPath string) (*gogit.Repository, error) {
	path := g.repoPath(repoPath)
	fs := osfs.New(path)
	s := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	repo, err := gogit.Open(s, nil)
	if err != nil {
		return nil, fmt.Errorf("gogit: open %s: %w", path, err)
	}
	return repo, nil
}
