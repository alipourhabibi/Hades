package gogit

import (
	"context"
	"errors"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func (g *GoGitStorage) ListCommits(_ context.Context, repoPath, ref string, limit int) ([]*git.CommitInfo, error) {
	if limit <= 0 {
		limit = git.DefaultCommitLimit
	}
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
	// The iterator holds open object-store handles, so it is closed rather than
	// left to the garbage collector.
	defer iter.Close()

	commits := make([]*git.CommitInfo, 0, limit)
	// storer.ErrStop ends the walk once limit commits are collected. Walking
	// the entire history, which is what this did, is unbounded work on a
	// request path for a repository with a long history.
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, &git.CommitInfo{
			SHA:       c.Hash.String(),
			Message:   c.Message,
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
		})
		if len(commits) >= limit {
			return storer.ErrStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, err
	}
	return commits, nil
}
