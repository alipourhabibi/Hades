package gogit

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *GoGitStorage) GetFile(_ context.Context, repoPath, ref, filePath string) ([]byte, int64, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, 0, err
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, 0, git.ErrNotFound
	}
	commit, err := repo.CommitObject(*h)
	if err != nil {
		return nil, 0, git.ErrNotFound
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, 0, err
	}
	f, err := tree.File(filePath)
	if err != nil {
		return nil, 0, git.ErrNotFound
	}
	content, err := f.Contents()
	if err != nil {
		return nil, 0, err
	}
	return []byte(content), int64(len(content)), nil
}

func (g *GoGitStorage) ListFiles(_ context.Context, repoPath, ref string) ([]string, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, git.ErrNotFound
	}
	commit, err := repo.CommitObject(*h)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	var paths []string
	tree.Files().ForEach(func(f *object.File) error {
		paths = append(paths, f.Name)
		return nil
	})
	return paths, nil
}

func (g *GoGitStorage) GetTreeEntries(_ context.Context, repoPath, ref, dir string) ([]*git.TreeEntry, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, git.ErrNotFound
	}
	commit, err := repo.CommitObject(*h)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	if dir != "" && dir != "." {
		subtree, err := tree.Tree(dir)
		if err != nil {
			return nil, git.ErrNotFound
		}
		tree = subtree
	}

	entries := make([]*git.TreeEntry, 0, len(tree.Entries))
	for _, e := range tree.Entries {
		t := git.TreeEntryTypeFile
		if e.Mode == filemode.Dir {
			t = git.TreeEntryTypeDir
		}
		fullPath := e.Name
		if dir != "" && dir != "." {
			fullPath = dir + "/" + e.Name
		}
		entries = append(entries, &git.TreeEntry{
			Name: e.Name,
			Path: fullPath,
			OID:  e.Hash.String(),
			Type: t,
			Mode: int32(e.Mode),
		})
	}
	return entries, nil
}
