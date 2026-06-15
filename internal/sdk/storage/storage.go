// Package storage defines the Backend interface for storing generated SDK
// artifacts. The only current implementation is the S3-compatible backend
// in the s3 sub-package.
package storage

import (
	"context"
	"io"
)

// File represents a generated SDK file (used for downloads).
type File struct {
	Path    string
	Content []byte
}

// Backend is the interface for storing generated SDK artifacts.
type Backend interface {
	// Upload streams all files from localDir to the object store under keyPrefix.
	// keyPrefix pattern: "<module_name>/<commit_id>/<language>"
	// The upload is idempotent: files already present at their object key are skipped.
	Upload(ctx context.Context, keyPrefix string, localDir string) (locationURI string, err error)
	// Download retrieves all generated SDK files for the given key prefix.
	Download(ctx context.Context, key string) ([]*File, error)
	// GetFile streams a single object by its exact key.
	// The caller is responsible for closing the returned ReadCloser.
	// Returns (nil, 0, ErrNotFound) when the key does not exist.
	GetFile(ctx context.Context, key string) (io.ReadCloser, int64, error)
}
