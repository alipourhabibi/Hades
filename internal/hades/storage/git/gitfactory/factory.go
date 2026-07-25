// Package gitfactory selects and constructs the git.Storage backend from config.
// It lives in its own package to avoid the import cycle that would arise if the
// parent git package imported its own sub-packages (gogit, gitaly), which both
// import git for the Storage interface and shared types.
package gitfactory

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	gitaly "github.com/alipourhabibi/Hades/internal/hades/storage/git/gitaly"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gogit"
)

// NewFromConfig constructs the git.Storage backend selected by cfg.Backends.Git.
// Defaults to gogit when the field is unset.
func NewFromConfig(c *config.Config) (git.Storage, error) {
	switch git.SelectedBackend(c.Backends) {
	case config.GitGitaly:
		svc, err := gitaly.NewStorage(c.Gitaly)
		if err != nil {
			return nil, fmt.Errorf("git: gitaly: %w", err)
		}
		return gitaly.New(
			svc.RepositoryService,
			svc.OperattionService,
			svc.BlobService,
			svc.CommitService,
			svc.TreeService,
			svc.DiffService,
		), nil
	default:
		root := c.GitStorage.Root
		if root == "" {
			root = "./data/repos"
		}
		return gogit.New(root), nil
	}
}
