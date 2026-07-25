package gogit

import (
	"bytes"
	"context"
	"strings"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *GoGitStorage) GetCommitDiff(_ context.Context, repoPath, commitHash string) ([]*git.FileDiff, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	h := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(h)
	if err != nil {
		return nil, git.ErrNotFound
	}

	var parentTree *object.Tree
	if len(commit.ParentHashes) > 0 {
		parent, err := repo.CommitObject(commit.ParentHashes[0])
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		return nil, err
	}

	var diffs []*git.FileDiff
	for _, c := range changes {
		from, to, err := c.Files()
		if err != nil {
			continue
		}

		fd := &git.FileDiff{}
		if from != nil {
			fd.FromPath = from.Name
		}
		if to != nil {
			fd.ToPath = to.Name
		}
		fd.IsNewFile = from == nil && to != nil
		fd.IsDeletedFile = from != nil && to == nil
		fd.IsRenamedFile = !fd.IsNewFile && !fd.IsDeletedFile && fd.FromPath != fd.ToPath

		patch, err := c.Patch()
		if err == nil {
			var buf bytes.Buffer
			patch.Encode(&buf)
			fd.Patch = buf.String()
			countLines(fd)
		}

		diffs = append(diffs, fd)
	}
	return diffs, nil
}

func countLines(fd *git.FileDiff) {
	for _, line := range strings.Split(fd.Patch, "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			if !strings.HasPrefix(line, "+++") {
				fd.Additions++
			}
		case '-':
			if !strings.HasPrefix(line, "---") {
				fd.Deletions++
			}
		}
	}
}

var _ git.Storage = (*GoGitStorage)(nil)
