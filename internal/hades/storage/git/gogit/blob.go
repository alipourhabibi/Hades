package gogit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/utils/paths"
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
	// The iteration error is returned. Discarding it, which is what this did,
	// turned a mid-iteration read failure into a silently truncated file set
	// with err == nil: a partially read module looked like a small module, and
	// the next push would have written that truncated set back.
	if err := tree.Files().ForEach(func(f *object.File) error {
		if err := paths.ValidatePath(f.Name); err != nil {
			return fmt.Errorf("gogit: refusing to read %q: %w", f.Name, err)
		}
		content, err := f.Contents()
		if err != nil {
			return err
		}
		files = append(files, &git.File{Path: f.Name, Content: []byte(content)})
		return nil
	}); err != nil {
		return nil, err
	}
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
		// Validate on the way out as well as on the way in. Anything already in
		// the object store is otherwise trusted when it is read back, and this
		// path writes to the filesystem: a tree entry named "../../etc/x"
		// would escape dir. Phase 8 closed the upload direction; this is the
		// read direction.
		if err := paths.ValidatePath(f.Name); err != nil {
			return fmt.Errorf("gogit: refusing to write %q: %w", f.Name, err)
		}
		destPath := filepath.Join(dir, f.Name)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return err
		}
		r, err := f.Reader()
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		// #nosec G304 -- the path is validated by paths.ValidatePath above and is rooted at dir.
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, r); err != nil {
			_ = out.Close()
			return err
		}
		// Close is checked: a buffered write that fails on flush would
		// otherwise produce a silently truncated file on disk.
		return out.Close()
	})
}
