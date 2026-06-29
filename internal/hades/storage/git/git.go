// Package git defines the Storage interface for all git versioning operations
// on proto schema repositories. Implementations include GoGitStorage (default,
// zero-dep) and GitalyStorage (production, wraps Gitaly gRPC).
package git

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested file, ref, or repository does not exist.
var ErrNotFound = errors.New("git: not found")

// TreeEntry represents one immediate child of a directory in the repository tree.
type TreeEntry struct {
	Name string
	Path string
	OID  string
	Type TreeEntryType
	Mode int32
}

// TreeEntryType distinguishes files from directories.
type TreeEntryType int

const (
	TreeEntryTypeFile TreeEntryType = iota
	TreeEntryTypeDir
)

// CommitInfo holds metadata about a single Git commit.
type CommitInfo struct {
	SHA       string
	Message   string
	Author    string
	Email     string
	Timestamp time.Time
}

// FileDiff holds the diff data for a single file between two refs.
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

// File is a path+content pair used for write operations.
type File struct {
	Path    string
	Content []byte
}

// Storage is the abstraction for git versioning of proto schema repositories.
// All handler code that previously accessed Gitaly sub-services directly
// should depend on this interface instead.
type Storage interface {
	// CreateRepository initialises a new repository at repoPath under the
	// configured root. defaultBranch is used as the initial branch name.
	CreateRepository(ctx context.Context, repoPath, defaultBranch string) error

	// DeleteRepository removes the repository at repoPath. Used as a
	// compensating action when a DB transaction fails after repository creation.
	DeleteRepository(ctx context.Context, repoPath string) error

	// PutFiles creates a commit on branch containing the given files.
	// existingPaths lists paths that already exist in the repo (to determine
	// create vs update). Returns the resulting commit SHA.
	PutFiles(ctx context.Context, repoPath, branch string, files []*File, authorName, authorEmail, commitMsg string, existingPaths []string) (commitSHA string, err error)

	// RollbackCommit resets branch back to previousHead. If previousHead is
	// empty the function is a no-op (orphan commit will be cleaned up separately).
	RollbackCommit(ctx context.Context, repoPath, branch, currentHead, previousHead string) error

	// GetFile returns the raw bytes of the file at filePath for the given ref.
	// Returns ErrNotFound when the path or ref does not exist.
	GetFile(ctx context.Context, repoPath, ref, filePath string) ([]byte, int64, error)

	// ListFiles returns all file paths in the repository at the given ref.
	ListFiles(ctx context.Context, repoPath, ref string) ([]string, error)

	// ListBlobs returns all files with their content at the given commitHash.
	ListBlobs(ctx context.Context, repoPath, commitHash string) ([]*File, error)

	// StreamBlobsToDir writes all blobs at commitHash to disk under dir,
	// streaming without buffering the full repo in memory.
	StreamBlobsToDir(ctx context.Context, repoPath, commitHash, dir string) error

	// GetTreeEntries returns the immediate children of dir at the given ref.
	// An empty dir lists the repository root.
	GetTreeEntries(ctx context.Context, repoPath, ref, dir string) ([]*TreeEntry, error)

	// ListCommits returns commits reachable from ref in reverse-chronological order.
	ListCommits(ctx context.Context, repoPath, ref string) ([]*CommitInfo, error)

	// GetCommitDiff returns per-file diffs for commitHash against its parent.
	GetCommitDiff(ctx context.Context, repoPath, commitHash string) ([]*FileDiff, error)
}
