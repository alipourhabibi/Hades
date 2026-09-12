// Package disk implements sdkstorage.Backend using the local filesystem.
// It is the default zero-dependency artifact storage backend for self-host deployments.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
)

// ErrNotFound is returned when a requested artifact key does not exist on disk.
//
// It wraps the interface-level sentinel so a caller can match either: this one
// when it cares that the disk backend is in use, sdkstorage.ErrNotFound when it
// only cares that the artifact is absent. The Go module proxy does the latter.
var ErrNotFound = fmt.Errorf("disk: %w", sdkstorage.ErrNotFound)

// ErrUnsafeKey is returned when a key would resolve outside the storage root.
var ErrUnsafeKey = errors.New("disk: key escapes the artifact root")

// DiskStorage stores SDK artifacts under a configured root directory.
// Keys are mapped to subdirectories: <root>/<key>/<filename>.
type DiskStorage struct {
	root string
}

// New creates a DiskStorage rooted at root.
func New(root string) *DiskStorage {
	if root == "" {
		// Underscore-prefixed so the go tool ignores the tree: generated SDKs are
		// Go source, and two of them in one directory can declare conflicting
		// package names, which breaks `go build ./...` for the whole module.
		root = "./_data/artifacts"
	}
	return &DiskStorage{root: root}
}

// resolve joins key onto the root and refuses anything that escapes it.
//
// Keys are built from user-controlled module names (see
// internal/sdk/worker.process, which formats "<module>/<commit>/<language>"),
// so "../../etc" in a module name would otherwise write outside the artifact
// root entirely. filepath.Join cleans the path, which is what makes the prefix
// comparison below meaningful.
func (d *DiskStorage) resolve(key string) (string, error) {
	root, err := filepath.Abs(d.root)
	if err != nil {
		return "", err
	}
	full, err := filepath.Abs(filepath.Join(root, key))
	if err != nil {
		return "", err
	}
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeKey, key)
	}
	return full, nil
}

// Upload writes all files from the in-memory map to <root>/<keyPrefix>/<file.Path>.
// Existing files at the same path are skipped (idempotent).
func (d *DiskStorage) Upload(_ context.Context, keyPrefix string, localDir string) (string, error) {
	destRoot, err := d.resolve(keyPrefix)
	if err != nil {
		return "", err
	}
	// Walk localDir and copy files into the key prefix directory.
	return destRoot, filepath.WalkDir(localDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destRoot, rel)
		if dest != destRoot && !strings.HasPrefix(dest, destRoot+string(os.PathSeparator)) {
			return fmt.Errorf("%w: %q", ErrUnsafeKey, rel)
		}
		if _, statErr := os.Stat(dest); statErr == nil {
			return nil // already exists, skip
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return err
		}
		// #nosec G304,G122 -- path comes from WalkDir over the caller-owned localDir, which this process created and owns for the life of the call.
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()
		// #nosec G304 -- dest is under destRoot, which resolve confirmed is inside the artifact root.
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			_ = out.Close()
			return err
		}
		// Close is checked: a failure to flush would leave a truncated
		// artifact that later reads would treat as complete.
		return out.Close()
	})
}

// Download retrieves all files stored under key and returns them as []*File.
func (d *DiskStorage) Download(_ context.Context, key string) ([]*sdkstorage.File, error) {
	dir, err := d.resolve(key)
	if err != nil {
		return nil, err
	}
	var files []*sdkstorage.File
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// #nosec G304,G122 -- the path comes from WalkDir under a root resolve confirmed is inside the artifact root.
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

// ListFiles returns the relative paths of every file stored under keyPrefix.
func (d *DiskStorage) ListFiles(_ context.Context, keyPrefix string) ([]string, error) {
	dir, err := d.resolve(keyPrefix)
	if err != nil {
		return nil, err
	}
	var paths []string
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	return paths, err
}

// GetFile streams a single artifact identified by its exact key path.
// Returns (nil, 0, ErrNotFound) when the key does not exist.
func (d *DiskStorage) GetFile(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := d.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	// #nosec G304 -- the key is checked by resolve, which refuses anything outside the artifact root.
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// Ensure DiskStorage implements sdkstorage.Backend at compile time.
var _ sdkstorage.Backend = (*DiskStorage)(nil)
