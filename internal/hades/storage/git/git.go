// Package git defines the Storage interface for all git versioning operations
// on proto schema repositories. Implementations include GoGitStorage (default,
// zero-dep) and GitalyStorage (production, wraps Gitaly gRPC).
package git

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alipourhabibi/Hades/utils/connerr"
)

// ErrNotFound is returned when a requested file, ref, or repository does not exist.
var ErrNotFound = fmt.Errorf("git: %w", connerr.ErrNotFound)

// ErrUnsafeRepoPath is returned when a repository path is not something this
// storage may address.
var ErrUnsafeRepoPath = errors.New("git: unsafe repository path")

// repoSegmentRe is one segment of a repository path: it must start with an
// alphanumeric, so "..", ".git" and "-x" cannot match.
var repoSegmentRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateRepoPath reports whether repoPath is safe to resolve against a
// storage root.
//
// This is the second of two defences and is deliberately redundant with the
// name validation in the handler. gogit resolves a repository with
// filepath.Join(root, repoPath), and Join calls Clean, which resolves ".."
// instead of refusing it, so an unvalidated module name of "../escape" put a
// repository outside its owner's namespace. Gitaly takes the same string as a
// RelativePath. A validation gap upstream should not be able to reach the
// filesystem, so the storage layer checks too.
//
// One or two segments: "owner/module" for a module, and a bare name for the
// shared SDK artifact repository.
func ValidateRepoPath(repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("%w: empty", ErrUnsafeRepoPath)
	}
	if strings.ContainsAny(repoPath, "\\\x00") {
		return fmt.Errorf("%w: %q contains a backslash or NUL", ErrUnsafeRepoPath, repoPath)
	}
	segments := strings.Split(repoPath, "/")
	if len(segments) > 2 {
		return fmt.Errorf("%w: %q has more than two segments", ErrUnsafeRepoPath, repoPath)
	}
	for _, s := range segments {
		if !repoSegmentRe.MatchString(s) {
			return fmt.Errorf("%w: %q contains an unusable segment %q", ErrUnsafeRepoPath, repoPath, s)
		}
	}
	return nil
}

// ErrRefMoved is returned by PutFiles when the branch no longer points at the
// commit the caller based its work on.
//
// The push path is a read-modify-write: the caller reads the latest commit,
// reads its files, merges, and writes. Without a compare-and-swap on the ref,
// two overlapping pushes each compute a tree from the same parent and the
// second silently discards the first, with both calls returning success and a
// valid database row for each. See docs2/adr/007.
var ErrRefMoved = errors.New("git: branch moved since it was read")

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

// DefaultCommitLimit bounds ListCommits when the caller names no limit.
const DefaultCommitLimit = 100

// File is a path+content pair used for write operations.
type File struct {
	Path    string
	Content []byte
}

// PutFilesRequest describes one commit to write.
//
// It is a struct rather than a positional argument list because the previous
// signature had eight of them and the one that mattered most, the existing
// paths, was silently discarded by one implementation and used only to pick
// between CREATE and UPDATE by the other. Neither could express a deletion, so
// a user who removed a .proto file and pushed got a successful push and the
// file was still there.
type PutFilesRequest struct {
	// RepoPath is the repository, in "owner/module" form.
	RepoPath string

	// Branch is the branch to move.
	Branch string

	// Files is the complete set of files the caller is authoritative for, with
	// their new content.
	Files []*File

	// ExistingPaths is the set of paths currently in the branch that the caller
	// is authoritative for. Any path listed here and absent from Files is
	// DELETED by this commit. Paths the caller is not authoritative for, such
	// as registry-generated metadata, must be left out of both lists and are
	// carried through untouched.
	ExistingPaths []string

	AuthorName  string
	AuthorEmail string

	// Message is the commit message.
	Message string

	// Digest is recorded alongside the commit. It is passed as a value rather
	// than embedded in Message and parsed back out: recovering a value from
	// your own free-text commit message is a bug waiting for the marker string
	// to drift, which is exactly what happened.
	Digest string

	// ExpectedHead is the commit the caller based its work on, or "" when the
	// caller expects the branch not to exist yet. The ref update is a
	// compare-and-swap against it; PutFiles returns ErrRefMoved if the branch
	// points anywhere else.
	ExpectedHead string
}

// Storage is the abstraction for git versioning of proto schema repositories.
// All handler code that previously accessed Gitaly sub-services directly
// should depend on this interface instead.
type Storage interface {
	// CreateRepository initialises a new repository at repoPath under the
	// configured root. defaultBranch is used as the initial branch name.
	//
	// It is idempotent: a repository that already exists is not an error. Both
	// callers want that. The module-creation saga can retry after a failure
	// that left the repository behind, and the SDK artifact backend calls this
	// on every upload to make sure its one shared repository is there.
	//
	// gogit has always behaved this way. Gitaly did not, and returned
	// AlreadyExists, which made the second SDK job ever run against a Gitaly
	// artifact store fail, and every job after it, permanently.
	CreateRepository(ctx context.Context, repoPath, defaultBranch string) error

	// DeleteRepository removes the repository at repoPath. Used as a
	// compensating action when a DB transaction fails after repository creation.
	DeleteRepository(ctx context.Context, repoPath string) error

	// PutFiles creates a commit on a branch and returns the resulting commit SHA.
	//
	// It returns ErrRefMoved when the branch has moved away from
	// req.ExpectedHead since the caller read it.
	PutFiles(ctx context.Context, req PutFilesRequest) (commitSHA string, err error)

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

	// ListCommits returns commits reachable from ref in reverse-chronological
	// order, at most limit of them. A limit of zero or less applies
	// DefaultCommitLimit: an unbounded history walk on a repository with a long
	// history is a request that never returns.
	ListCommits(ctx context.Context, repoPath, ref string, limit int) ([]*CommitInfo, error)

	// GetCommitDiff returns per-file diffs for commitHash against its parent.
	GetCommitDiff(ctx context.Context, repoPath, commitHash string) ([]*FileDiff, error)

	// Close releases any resources the backend holds, such as gRPC connections.
	// It is safe to call on a backend that holds none.
	Close() error
}
