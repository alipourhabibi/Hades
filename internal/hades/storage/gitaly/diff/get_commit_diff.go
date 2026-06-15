package diff

import (
	"bytes"
	"context"
	"io"
	"strings"

	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

// emptyTreeSHA is the well-known git SHA1 for an empty tree. Used as the
// left-hand side when diffing the very first commit (which has no parent).
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// FileDiff holds the diff data for a single file.
type FileDiff struct {
	FromPath      string
	ToPath        string
	IsNewFile     bool
	IsDeletedFile bool
	IsRenamedFile bool
	Additions     int32
	Deletions     int32
	Patch         string
	Binary        bool
	TooLarge      bool
}

// GetCommitDiff returns the per-file diffs for the given commit hash in the
// repository identified by owner/module.  For initial commits (no parent) the
// diff is computed against the empty tree.
func (s *DiffService) GetCommitDiff(ctx context.Context, owner, module, commitHash string) ([]*FileDiff, error) {
	repo := &pb.Repository{
		StorageName:   s.defaultStorageName,
		RelativePath:  owner + "/" + module,
		GlRepository:  owner + "/" + module,
	}

	// Resolve the parent commit hash.
	findResp, err := s.commitClient.FindCommit(ctx, &pb.FindCommitRequest{
		Repository: repo,
		Revision:   []byte(commitHash),
	})
	if err != nil {
		return nil, err
	}

	leftID := emptyTreeSHA
	if findResp != nil && findResp.Commit != nil && len(findResp.Commit.ParentIds) > 0 {
		leftID = findResp.Commit.ParentIds[0]
	}

	// Stream the commit diff from Gitaly.
	stream, err := s.diffClient.CommitDiff(ctx, &pb.CommitDiffRequest{
		Repository:    repo,
		LeftCommitId:  leftID,
		RightCommitId: commitHash,
	})
	if err != nil {
		return nil, err
	}

	// Gitaly may split a single file's patch across multiple streaming chunks.
	// We accumulate RawPatchData until EndOfPatch == true.
	var diffs []*FileDiff
	var current *FileDiff
	var patchBuf bytes.Buffer

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// A new file always starts a fresh chunk where FromPath/ToPath are set.
		// Continuation chunks for the same file have empty path fields.
		if current == nil || (len(chunk.FromPath) > 0 || len(chunk.ToPath) > 0) {
			// Flush the previous file if any.
			if current != nil {
				current.Patch = patchBuf.String()
				diffs = append(diffs, current)
				patchBuf.Reset()
			}
			// Derive file-status flags from mode fields:
			//   OldMode == 0  → new file (didn't exist before)
			//   NewMode == 0  → deleted file (doesn't exist after)
			//   Both non-zero, paths differ → renamed file
			isNew := chunk.OldMode == 0 && chunk.NewMode != 0
			isDel := chunk.NewMode == 0 && chunk.OldMode != 0
			isRen := !isNew && !isDel && string(chunk.FromPath) != string(chunk.ToPath) &&
				len(chunk.FromPath) > 0 && len(chunk.ToPath) > 0
			current = &FileDiff{
				FromPath:      string(chunk.FromPath),
				ToPath:        string(chunk.ToPath),
				IsNewFile:     isNew,
				IsDeletedFile: isDel,
				IsRenamedFile: isRen,
				Binary:        chunk.Binary,
				TooLarge:      chunk.TooLarge,
			}
		}

		if !chunk.Binary && !chunk.TooLarge {
			patchBuf.Write(chunk.RawPatchData)
		}

		if chunk.EndOfPatch {
			current.Patch = patchBuf.String()
			countLines(current)
			diffs = append(diffs, current)
			patchBuf.Reset()
			current = nil
		}
	}

	// Handle trailing file (no EndOfPatch received).
	if current != nil {
		current.Patch = patchBuf.String()
		countLines(current)
		diffs = append(diffs, current)
	}

	return diffs, nil
}

// countLines tallies added and deleted lines in a unified diff patch.
func countLines(fd *FileDiff) {
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
