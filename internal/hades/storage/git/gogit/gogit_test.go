package gogit_test

import (
	"context"
	"os"
	"testing"

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

	commitSHA, err := s.PutFiles(ctx, "owner/repo", "main", []*git.File{
		{Path: "hello.proto", Content: []byte("syntax = \"proto3\";")},
	}, "Test User", "test@example.com", "initial commit", nil)
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
	if err != git.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListFiles(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	_ = s.CreateRepository(ctx, "owner/repo", "main")
	_, err := s.PutFiles(ctx, "owner/repo", "main", []*git.File{
		{Path: "a.proto", Content: []byte("a")},
		{Path: "b.proto", Content: []byte("b")},
	}, "u", "u@x.com", "commit", nil)
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
	sha, _ := s.PutFiles(ctx, "owner/repo", "main", []*git.File{
		{Path: "x.proto", Content: []byte("x")},
	}, "u", "u@x.com", "commit", nil)

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
	_, _ = s.PutFiles(ctx, "owner/repo", "main", []*git.File{{Path: "a.proto", Content: []byte("a")}}, "u", "u@x.com", "first", nil)
	_, _ = s.PutFiles(ctx, "owner/repo", "main", []*git.File{{Path: "b.proto", Content: []byte("b")}}, "u", "u@x.com", "second", []string{"a.proto"})

	commits, err := s.ListCommits(ctx, "owner/repo", "main")
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
	_, _ = s.PutFiles(ctx, "owner/repo", "main", []*git.File{{Path: "a.proto", Content: []byte("a")}}, "u", "u@x.com", "c", nil)

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
