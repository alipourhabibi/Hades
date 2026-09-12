package gogit_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gogit"
)

func newTestStorage(t *testing.T) (*gogit.GoGitStorage, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "gogit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return gogit.New(root), root
}

func TestCreateAndGetFile(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	if err := s.CreateRepository(ctx, "owner/repo", "main"); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	commitSHA, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath:    "owner/repo",
		Branch:      "main",
		Files:       []*git.File{{Path: "hello.proto", Content: []byte("syntax = \"proto3\";")}},
		AuthorName:  "Test User",
		AuthorEmail: "test@example.com",
		Message:     "initial commit",
	})
	if err != nil {
		t.Fatalf("PutFiles: %v", err)
	}
	if commitSHA == "" {
		t.Fatal("expected non-empty commit SHA")
	}

	content, _, err := s.GetFile(ctx, "owner/repo", "main", "hello.proto")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if string(content) != "syntax = \"proto3\";" {
		t.Fatalf("GetFile: got %q, want %q", content, "syntax = \"proto3\";")
	}
}

func TestGetFile_NotFound(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	_, _, err := s.GetFile(ctx, "owner/repo", "main", "missing.proto")
	if !errors.Is(err, git.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListFiles(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	_, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo",
		Branch:   "main",
		Files: []*git.File{
			{Path: "a.proto", Content: []byte("a")},
			{Path: "b.proto", Content: []byte("b")},
		},
		AuthorName:  "u",
		AuthorEmail: "u@x.com",
		Message:     "commit",
	})
	if err != nil {
		t.Fatalf("PutFiles: %v", err)
	}

	files, err := s.ListFiles(ctx, "owner/repo", "main")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ListFiles: got %d files, want 2", len(files))
	}
}

func TestListBlobs(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	sha, _ := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath:    "owner/repo",
		Branch:      "main",
		Files:       []*git.File{{Path: "x.proto", Content: []byte("x")}},
		AuthorName:  "u",
		AuthorEmail: "u@x.com",
		Message:     "commit",
	})

	blobs, err := s.ListBlobs(ctx, "owner/repo", sha)
	if err != nil {
		t.Fatalf("ListBlobs: %v", err)
	}
	if len(blobs) != 1 || blobs[0].Path != "x.proto" {
		t.Fatalf("ListBlobs: unexpected result %v", blobs)
	}
}

func TestListCommits(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	first, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:       []*git.File{{Path: "a.proto", Content: []byte("a")}},
		AuthorName:  "u",
		AuthorEmail: "u@x.com",
		Message:     "first",
	})
	if err != nil {
		t.Fatalf("PutFiles: %v", err)
	}
	// ExistingPaths names a.proto and Files does not, so this commit deletes it.
	if _, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:         []*git.File{{Path: "b.proto", Content: []byte("b")}},
		ExistingPaths: []string{"a.proto"},
		AuthorName:    "u",
		AuthorEmail:   "u@x.com",
		Message:       "second",
		ExpectedHead:  first,
	}); err != nil {
		t.Fatalf("PutFiles: %v", err)
	}

	commits, err := s.ListCommits(ctx, "owner/repo", "main", 0)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("ListCommits: got %d commits, want >=2", len(commits))
	}
}

func TestGetTreeEntries(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	_, _ = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:       []*git.File{{Path: "a.proto", Content: []byte("a")}},
		AuthorName:  "u",
		AuthorEmail: "u@x.com",
		Message:     "c",
	})

	entries, err := s.GetTreeEntries(ctx, "owner/repo", "main", "")
	if err != nil {
		t.Fatalf("GetTreeEntries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("GetTreeEntries: expected at least one entry")
	}
}

func TestDeleteRepository(t *testing.T) {
	s, root := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	if err := s.DeleteRepository(ctx, "owner/repo"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	if _, err := os.Stat(root + "/owner/repo"); !os.IsNotExist(err) {
		t.Fatal("expected repo directory to be gone after DeleteRepository")
	}
}

// TestPutFilesDeletesPathsNotInTheNewSet is the regression test for a file
// never being removable from a module: PutFiles discarded its existing-paths
// argument and seeded the tree from the parent, so it could only ever add.
func TestPutFilesDeletesPathsNotInTheNewSet(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()
	require.NoError(t, s.CreateRepository(ctx, "owner/repo", "main"))

	first, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files: []*git.File{
			{Path: "keep.proto", Content: []byte("keep")},
			{Path: "drop.proto", Content: []byte("drop")},
			{Path: "nested/also.proto", Content: []byte("nested")},
		},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "first",
	})
	require.NoError(t, err)

	_, err = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:         []*git.File{{Path: "keep.proto", Content: []byte("keep")}},
		ExistingPaths: []string{"keep.proto", "drop.proto", "nested/also.proto"},
		AuthorName:    "u", AuthorEmail: "u@x.com", Message: "second",
		ExpectedHead: first,
	})
	require.NoError(t, err)

	files, err := s.ListFiles(ctx, "owner/repo", "main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"keep.proto"}, files,
		"a path in ExistingPaths and absent from Files must be deleted")
}

// TestPutFilesLeavesUnlistedPathsAlone pins the other half of the contract: a
// path the caller is not authoritative for, so listed in neither set, survives.
// buf.yaml is registry-generated and relies on this.
func TestPutFilesLeavesUnlistedPathsAlone(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()
	require.NoError(t, s.CreateRepository(ctx, "owner/repo", "main"))

	first, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files: []*git.File{
			{Path: "a.proto", Content: []byte("a")},
			{Path: "buf.yaml", Content: []byte("version: v2")},
		},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "first",
	})
	require.NoError(t, err)

	_, err = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:         []*git.File{{Path: "b.proto", Content: []byte("b")}},
		ExistingPaths: []string{"a.proto"},
		AuthorName:    "u", AuthorEmail: "u@x.com", Message: "second",
		ExpectedHead: first,
	})
	require.NoError(t, err)

	files, err := s.ListFiles(ctx, "owner/repo", "main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"b.proto", "buf.yaml"}, files)
}

// TestPutFilesRefusesAStaleExpectedHead covers the compare-and-swap: two pushes
// computed from the same parent must not both succeed, because the second's
// tree does not contain the first's work and would discard it.
func TestPutFilesRefusesAStaleExpectedHead(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()
	require.NoError(t, s.CreateRepository(ctx, "owner/repo", "main"))

	base, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:      []*git.File{{Path: "a.proto", Content: []byte("a")}},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "base",
	})
	require.NoError(t, err)

	// Two writers both read `base` and both compute a tree from it.
	_, err = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:      []*git.File{{Path: "b.proto", Content: []byte("b")}},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "writer one",
		ExpectedHead: base,
	})
	require.NoError(t, err, "the first writer wins")

	_, err = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:      []*git.File{{Path: "c.proto", Content: []byte("c")}},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "writer two",
		ExpectedHead: base,
	})
	require.ErrorIs(t, err, git.ErrRefMoved, "the second writer must be refused, not silently discard the first")

	files, err := s.ListFiles(ctx, "owner/repo", "main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.proto", "b.proto"}, files,
		"the first writer's work must still be there")
}

// TestPutFilesRefusesToCreateOverAnExistingBranch is the empty-ExpectedHead
// half of the same rule.
func TestPutFilesRefusesToCreateOverAnExistingBranch(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()
	require.NoError(t, s.CreateRepository(ctx, "owner/repo", "main"))

	_, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:      []*git.File{{Path: "a.proto", Content: []byte("a")}},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "first",
	})
	require.NoError(t, err)

	_, err = s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files:      []*git.File{{Path: "b.proto", Content: []byte("b")}},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "second",
	})
	require.ErrorIs(t, err, git.ErrRefMoved,
		"an empty ExpectedHead means the branch must not exist yet")
}

// TestTreeEntriesAreSortedTheWayGitSortsThem covers the canonical ordering: git
// compares a directory entry as though its name ended in a slash, so a plain
// name comparison produces a tree `git fsck` reports as not properly sorted and
// whose hash differs from canonical git's for identical content.
func TestTreeEntriesAreSortedTheWayGitSortsThem(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()
	require.NoError(t, s.CreateRepository(ctx, "owner/repo", "main"))

	// "foo.proto" sorts before "foo/" but after "foo", so the two orderings
	// disagree on exactly this shape.
	_, err := s.PutFiles(ctx, git.PutFilesRequest{
		RepoPath: "owner/repo", Branch: "main",
		Files: []*git.File{
			{Path: "foo.proto", Content: []byte("a")},
			{Path: "foo/bar.proto", Content: []byte("b")},
		},
		AuthorName: "u", AuthorEmail: "u@x.com", Message: "c",
	})
	require.NoError(t, err)

	entries, err := s.GetTreeEntries(ctx, "owner/repo", "main", "")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "foo.proto", entries[0].Name)
	assert.Equal(t, "foo", entries[1].Name)
}
