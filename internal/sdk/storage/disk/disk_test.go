package disk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alipourhabibi/Hades/internal/sdk/storage/disk"
)

func newTestStorage(t *testing.T) (*disk.DiskStorage, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "disk-sdk-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return disk.New(root), root
}

func TestDiskStorage_UploadDownload(t *testing.T) {
	s, root := newTestStorage(t)
	ctx := context.Background()

	// Create a temp dir with a file to upload.
	srcDir, err := os.MkdirTemp("", "disk-sdk-src-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	testFile := filepath.Join(srcDir, "output.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	key := "mymodule/abc123/go"
	loc, err := s.Upload(ctx, key, srcDir)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if loc == "" {
		t.Fatal("expected non-empty location URI")
	}

	// Verify file exists on disk.
	destPath := filepath.Join(root, key, "output.go")
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("expected file at %s: %v", destPath, err)
	}

	files, err := s.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(files) != 1 || files[0].Path != "output.go" {
		t.Fatalf("Download: unexpected files %v", files)
	}
}

func TestDiskStorage_GetFile(t *testing.T) {
	s, root := newTestStorage(t)
	ctx := context.Background()

	// Write a file manually.
	keyPath := filepath.Join(root, "mod", "hash", "go", "main.go")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	rc, size, err := s.GetFile(ctx, "mod/hash/go/main.go")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	defer rc.Close()
	if size != 5 {
		t.Fatalf("GetFile: size %d, want 5", size)
	}
}

func TestDiskStorage_GetFile_NotFound(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	rc, _, err := s.GetFile(ctx, "nonexistent/path/file.go")
	if err != disk.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v (rc=%v)", err, rc)
	}
}

func TestDiskStorage_UploadIdempotent(t *testing.T) {
	s, _ := newTestStorage(t)
	ctx := context.Background()

	srcDir, err := os.MkdirTemp("", "disk-sdk-src2-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)
	if err := os.WriteFile(filepath.Join(srcDir, "f.go"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Upload(ctx, "key", srcDir); err != nil {
		t.Fatal(err)
	}
	// Second upload should not fail.
	if _, err := s.Upload(ctx, "key", srcDir); err != nil {
		t.Fatalf("second Upload: %v", err)
	}
}
