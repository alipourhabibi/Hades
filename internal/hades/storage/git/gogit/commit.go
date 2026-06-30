package gogit

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *GoGitStorage) ListCommits(_ context.Context, repoPath, ref string) ([]*git.CommitInfo, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, git.ErrNotFound
	}
	iter, err := repo.Log(&gogit.LogOptions{From: *h})
	if err != nil {
		return nil, err
	}
	var commits []*git.CommitInfo
	iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, &git.CommitInfo{
			SHA:       c.Hash.String(),
			Message:   c.Message,
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
		})
		return nil
	})
	return commits, nil
}
