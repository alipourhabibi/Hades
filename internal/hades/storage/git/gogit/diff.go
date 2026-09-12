package gogit

import (
	"bytes"
	"context"
	"fmt"
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
		if err != nil {
			return nil, fmt.Errorf("gogit: read parent commit %s: %w", commit.ParentHashes[0], err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("gogit: read parent tree: %w", err)
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
			return nil, fmt.Errorf("gogit: read changed files: %w", err)
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

		// Binary and TooLarge are set here as well as in the Gitaly backend.
		// Leaving them unset meant the same API returned different diff
		// metadata depending on which backend was configured, and a client
		// that branches on Binary rendered a binary blob as text.
		fd.Binary = isBinaryBlob(from) || isBinaryBlob(to)
		fd.TooLarge = blobSize(from) > maxDiffBlobBytes || blobSize(to) > maxDiffBlobBytes

		if fd.Binary || fd.TooLarge {
			// No patch is produced for either, matching Gitaly.
			diffs = append(diffs, fd)
			continue
		}

		patch, err := c.Patch()
		if err != nil {
			return nil, fmt.Errorf("gogit: build patch for %s: %w", fd.ToPath, err)
		}
		var buf bytes.Buffer
		if err := patch.Encode(&buf); err != nil {
			return nil, fmt.Errorf("gogit: encode patch for %s: %w", fd.ToPath, err)
		}
		fd.Patch = buf.String()
		countLines(fd)

		diffs = append(diffs, fd)
	}
	return diffs, nil
}

// maxDiffBlobBytes is the size above which a file is reported as too large to
// diff rather than being rendered. It matches Gitaly's default patch limit.
const maxDiffBlobBytes = 100 * 1024

// isBinaryBlob reports whether the file's content is binary. A nil file, which
// is what a creation or deletion has on one side, is not binary by itself.
func isBinaryBlob(f *object.File) bool {
	if f == nil {
		return false
	}
	binary, err := f.IsBinary()
	if err != nil {
		// Unreadable content is treated as binary: producing a text patch from
		// bytes that could not be read is the worse answer.
		return true
	}
	return binary
}

func blobSize(f *object.File) int64 {
	if f == nil {
		return 0
	}
	return f.Size
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
