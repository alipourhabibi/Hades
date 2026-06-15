// Package blob wraps the Gitaly BlobService gRPC client, providing blob
// retrieval and streaming operations for module content.
package blob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BlobService wraps the Gitaly BlobService gRPC client.
type BlobService struct {
	client             pb.BlobServiceClient
	defaultStorageName string
}

// NewDefault dials the Gitaly server and returns a BlobService.
func NewDefault(c config.Gitaly) (*BlobService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", func() string { if c.Host != "" { return c.Host }; return "localhost" }(), c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewBlobServiceClient(conn)
	return &BlobService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// ListBlobs returns all files at the given commit revision as in-memory blobs.
func (b *BlobService) ListBlobs(ctx context.Context, commit *registryv1.Commit) ([]*registryv1.DownloadResponseContent, error) {
	moduleName := commit.Module.Name
	stream, err := b.client.ListBlobs(ctx, &pb.ListBlobsRequest{
		Revisions: []string{
			commit.CommitHash + ":",
		},
		Repository: &pb.Repository{
			StorageName:  b.defaultStorageName,
			RelativePath: moduleName,
			GlRepository: moduleName,
		},
		WithPaths:  true,
		BytesLimit: -1,
	})

	if err != nil {
		return nil, err
	}

	// Gitaly may split large blobs across multiple streaming messages. The
	// first chunk of each blob carries a non-zero Size (and the Path); all
	// subsequent chunks for the same blob have Size==0. We therefore keep a
	// single map across the entire stream, keyed by the most-recently seen
	// path, so that continuation chunks are appended to the right entry.
	mapFiles := map[string]*registryv1.File{}
	var lastPath string

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error receiving blob: %w", err)
		}

		for _, blob := range msg.Blobs {
			if blob.Size != 0 {
				// First chunk: carries path and metadata.
				lastPath = string(blob.GetPath())
				mapFiles[lastPath] = &registryv1.File{
					Path:    lastPath,
					Content: blob.Data,
				}
			} else if len(blob.Data) != 0 {
				// Continuation chunk: append to the current blob.
				mapFiles[lastPath].Content = append(mapFiles[lastPath].Content, blob.Data...)
			}
			// A chunk with Size==0 and no Data is an empty blob; already
			// handled by the first-chunk branch when Size was set.
		}
	}

	files := make([]*registryv1.File, 0, len(mapFiles))
	for _, f := range mapFiles {
		files = append(files, f)
	}

	contents := []*registryv1.DownloadResponseContent{
		{
			Commit: commit,
			Files:  files,
		},
	}

	return contents, nil
}

// StreamBlobsToDir fetches the blobs for the given commit from Gitaly and
// writes each file directly to disk under dir as chunks arrive, without
// buffering the entire repo in memory.
//
// Peak memory per call ≈ one gRPC frame (~64 KB), not the full repo size.
// The existing ListBlobs method is kept for other callers that need the
// in-memory representation (e.g. download, upload handlers).
func (b *BlobService) StreamBlobsToDir(ctx context.Context, commit *registryv1.Commit, dir string) error {
	moduleName := commit.Module.Name
	stream, err := b.client.ListBlobs(ctx, &pb.ListBlobsRequest{
		Revisions: []string{commit.CommitHash + ":"},
		Repository: &pb.Repository{
			StorageName:  b.defaultStorageName,
			RelativePath: moduleName,
			GlRepository: moduleName,
		},
		WithPaths:  true,
		BytesLimit: -1,
	})
	if err != nil {
		return err
	}

	var currentFile *os.File
	var currentPath string

	closeCurrentFile := func() error {
		if currentFile != nil {
			if err := currentFile.Close(); err != nil {
				return fmt.Errorf("closing %s: %w", currentPath, err)
			}
			currentFile = nil
			currentPath = ""
		}
		return nil
	}

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = closeCurrentFile()
			return fmt.Errorf("streaming blobs: %w", err)
		}

		for _, blob := range msg.Blobs {
			if blob.Size != 0 {
				// First chunk of a new blob: close the previous file and open a
				// new one at the correct path inside dir.
				if err := closeCurrentFile(); err != nil {
					return err
				}
				currentPath = string(blob.GetPath())
				destPath := filepath.Join(dir, currentPath)
				if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
					return fmt.Errorf("mkdir for %s: %w", currentPath, err)
				}
				currentFile, err = os.Create(destPath)
				if err != nil {
					return fmt.Errorf("create %s: %w", currentPath, err)
				}
				if len(blob.Data) > 0 {
					if _, err := currentFile.Write(blob.Data); err != nil {
						_ = currentFile.Close()
						return fmt.Errorf("write %s: %w", currentPath, err)
					}
				}
			} else if len(blob.Data) > 0 {
				// Continuation chunk: append to the currently open file.
				if currentFile == nil {
					return fmt.Errorf("received continuation chunk with no open file")
				}
				if _, err := currentFile.Write(blob.Data); err != nil {
					_ = currentFile.Close()
					return fmt.Errorf("write continuation %s: %w", currentPath, err)
				}
			}
		}
	}

	return closeCurrentFile()
}
