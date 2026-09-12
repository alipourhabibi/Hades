// Package gogit implements git.Storage using go-git with local bare repositories.
// It is the default zero-dependency git backend for self-hosted deployments.
package gogit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
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
//
// The default is underscore-prefixed so the go tool ignores the tree. Generated
// artifacts live beside it under the same parent, and two of them in one
// directory can declare conflicting package names, which breaks
// `go build ./...` for the whole module.
func New(root string) *GoGitStorage {
	if root == "" {
		root = "./_data/repos"
	}
	return &GoGitStorage{Root: root}
}

// Close satisfies git.Storage. The go-git backend holds no long-lived handles:
// every operation opens the repository it needs and releases it.
func (g *GoGitStorage) Close() error { return nil }

// repoPath resolves a repository path against the storage root.
//
// It validates first. filepath.Join calls Clean, which resolves ".." rather
// than refusing it, so Join(root, "owner/../escape") is root/escape: without
// the check, a module name that reached here unvalidated placed a repository
// outside its owner's namespace. See git.ValidateRepoPath.
func (g *GoGitStorage) repoPath(repoPath string) (string, error) {
	if err := git.ValidateRepoPath(repoPath); err != nil {
		return "", err
	}
	return filepath.Join(g.Root, repoPath), nil
}

// RepoRoot returns the directory holding every repository, resolved to an
// absolute path so a log line naming it is unambiguous about where the process
// is actually looking.
func (g *GoGitStorage) RepoRoot() string {
	abs, err := filepath.Abs(g.Root)
	if err != nil {
		return g.Root
	}
	return abs
}

// RepositoryExists reports whether repoPath names a repository this backend can
// open. A repository that is present but unreadable is reported as an error
// rather than as absent, so a permissions problem is not mistaken for a missing
// module.
func (g *GoGitStorage) RepositoryExists(_ context.Context, repoPath string) (bool, error) {
	if _, err := g.openRepo(repoPath); err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (g *GoGitStorage) openRepo(repoPath string) (*gogit.Repository, error) {
	path, err := g.repoPath(repoPath)
	if err != nil {
		return nil, err
	}
	fs := osfs.New(path)
	s := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	repo, err := gogit.Open(s, nil)
	if err != nil {
		return nil, fmt.Errorf("gogit: open %s: %w", path, err)
	}
	return repo, nil
}
