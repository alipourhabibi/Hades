// Package disk implements sdkstorage.Backend using the local filesystem.
// It is the default zero-dependency artifact storage backend for self-host deployments.
package disk

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
)

// ErrNotFound is returned when a requested artifact key does not exist on disk.
var ErrNotFound = errors.New("disk: not found")

// DiskStorage stores SDK artifacts under a configured root directory.
// Keys are mapped to subdirectories: <root>/<key>/<filename>.
type DiskStorage struct {
	root string
}

// New creates a DiskStorage rooted at root.
func New(root string) *DiskStorage {
	if root == "" {
		root = "./data/artifacts"
	}
	return &DiskStorage{root: root}
}

func (d *DiskStorage) keyDir(key string) string {
	return filepath.Join(d.root, key)
}

// Upload writes all files from the in-memory map to <root>/<keyPrefix>/<file.Path>.
// Existing files at the same path are skipped (idempotent).
func (d *DiskStorage) Upload(_ context.Context, keyPrefix string, localDir string) (string, error) {
	// Walk localDir and copy files into the key prefix directory.
	return filepath.Join(d.root, keyPrefix), filepath.WalkDir(localDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(d.root, keyPrefix, rel)
		if _, statErr := os.Stat(dest); statErr == nil {
			return nil // already exists, skip
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, src)
		return err
	})
}

// Download retrieves all files stored under key and returns them as []*File.
func (d *DiskStorage) Download(_ context.Context, key string) ([]*sdkstorage.File, error) {
	dir := d.keyDir(key)
	var files []*sdkstorage.File
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, &sdkstorage.File{Path: rel, Content: content})
		return nil
	})
	return files, err
}

// GetFile streams a single artifact identified by its exact key path.
// Returns (nil, 0, ErrNotFound) when the key does not exist.
func (d *DiskStorage) GetFile(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path := filepath.Join(d.root, key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// Ensure DiskStorage implements sdkstorage.Backend at compile time.
var _ sdkstorage.Backend = (*DiskStorage)(nil)
