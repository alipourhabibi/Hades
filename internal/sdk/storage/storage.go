// Package storage defines the Backend interface for storing generated SDK
// artifacts. The only current implementation is the S3-compatible backend
// in the s3 sub-package.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is what every Backend returns when a key does not exist.
//
// It is here, on the interface, rather than per backend, because the caller
// that matters is the Go module proxy: it distinguishes "the generator did not
// emit this file", which is a fallback, from "the store is unreachable", which
// is a 502. With no shared sentinel that classification was a substring match
// on the error message, and the S3 backend, which reported a missing object as
// io.ErrUnexpectedEOF, failed it silently: every .mod request returned 502 and
// no generated SDK could be resolved by go get on an S3 deployment.
var ErrNotFound = errors.New("sdk storage: artifact not found")

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
	//
	// Every file is held in memory at once. Prefer ListFiles plus GetFile when
	// the result is streamed to a client rather than inspected as a whole.
	Download(ctx context.Context, key string) ([]*File, error)
	// ListFiles returns the relative paths of every file stored under keyPrefix,
	// without reading any content.
	//
	// Paired with GetFile this lets a caller stream a large artifact set with
	// bounded memory: the Go module proxy builds a zip whose size is otherwise
	// proportional to the SDK size times the number of concurrent downloads.
	ListFiles(ctx context.Context, keyPrefix string) ([]string, error)
	// GetFile streams a single object by its exact key.
	// The caller is responsible for closing the returned ReadCloser.
	// Returns (nil, 0, ErrNotFound) when the key does not exist.
	GetFile(ctx context.Context, key string) (io.ReadCloser, int64, error)
}
