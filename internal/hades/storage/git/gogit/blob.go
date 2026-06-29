package gogit

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (g *GoGitStorage) ListBlobs(_ context.Context, repoPath, commitHash string) ([]*git.File, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	h := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(h)
	if err != nil {
		return nil, git.ErrNotFound
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	var files []*git.File
	tree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return err
		}
		files = append(files, &git.File{Path: f.Name, Content: []byte(content)})
		return nil
	})
	return files, nil
}

func (g *GoGitStorage) StreamBlobsToDir(_ context.Context, repoPath, commitHash, dir string) error {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return err
	}
	h := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(h)
	if err != nil {
		return git.ErrNotFound
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	return tree.Files().ForEach(func(f *object.File) error {
		destPath := filepath.Join(dir, f.Name)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}
		r, err := f.Reader()
		if err != nil {
			return err
		}
		defer r.Close()
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, r)
		return err
	})
}
