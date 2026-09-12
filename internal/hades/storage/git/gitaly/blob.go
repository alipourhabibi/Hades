package gitaly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/paths"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
)

// BlobService wraps the Gitaly BlobService gRPC client.
type BlobService struct {
	client             pb.BlobServiceClient
	defaultStorageName string
}

func newBlobService(conn *grpc.ClientConn, c config.Gitaly) *BlobService {
	return &BlobService{
		client:             pb.NewBlobServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}
}

// ListBlobs returns all files at the given commit revision as in-memory blobs.
func (b *BlobService) ListBlobs(ctx context.Context, commit *registryv1.Commit) ([]*registryv1.DownloadResponseContent, error) {
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
		return nil, err
	}

	// The discriminator between "a new file starts here" and "this is more of
	// the previous file" is the presence of the path field, not the size.
	//
	// Using size meant a legitimate zero-byte file was treated as a
	// continuation chunk, so it was either dropped or its (empty) data was
	// appended to the previous file, and if the very first message of a stream
	// was a continuation chunk then lastPath was still "" and the map lookup
	// returned nil, which is a nil-pointer panic on a request path.
	mapFiles := map[string]*registryv1.File{}
	var order []string
	var lastPath string

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error receiving blob: %w", err)
		}

		for _, blob := range msg.Blobs {
			if path := string(blob.GetPath()); path != "" {
				if err := paths.ValidatePath(path); err != nil {
					return nil, fmt.Errorf("gitaly: refusing blob %q: %w", path, err)
				}
				lastPath = path
				if _, seen := mapFiles[lastPath]; !seen {
					order = append(order, lastPath)
				}
				mapFiles[lastPath] = &registryv1.File{
					Path:    lastPath,
					Content: append([]byte(nil), blob.Data...),
				}
				continue
			}
			if len(blob.Data) == 0 {
				continue
			}
			f, ok := mapFiles[lastPath]
			if !ok {
				return nil, fmt.Errorf("gitaly: continuation chunk with no preceding path")
			}
			f.Content = append(f.Content, blob.Data...)
		}
	}

	// Deterministic order, matching the order Gitaly streamed them in. Ranging
	// over the map returned a different order on every call.
	files := make([]*registryv1.File, 0, len(order))
	for _, p := range order {
		files = append(files, mapFiles[p])
	}

	return []*registryv1.DownloadResponseContent{{Commit: commit, Files: files}}, nil
}

// StreamBlobsToDir fetches blobs for the given commit and writes each file
// directly to disk under dir as chunks arrive, without buffering the entire repo.
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
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = closeCurrentFile()
			return fmt.Errorf("streaming blobs: %w", err)
		}

		for _, blob := range msg.Blobs {
			// Presence of the path, not the size, marks a new file. See
			// ListBlobs: a zero-byte file is a real file.
			if path := string(blob.GetPath()); path != "" {
				if err := closeCurrentFile(); err != nil {
					return err
				}
				// Validate on read-back too. Anything already in the object
				// store was otherwise trusted, and this writes to the
				// filesystem, so a tree entry named "../../etc/x" would escape
				// dir.
				if err := paths.ValidatePath(path); err != nil {
					return fmt.Errorf("gitaly: refusing to write %q: %w", path, err)
				}
				currentPath = path
				destPath := filepath.Join(dir, currentPath)
				if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
					return fmt.Errorf("mkdir for %s: %w", currentPath, err)
				}
				// #nosec G304 -- the path is validated by paths.ValidatePath a few lines above and is rooted at dir.
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
				continue
			}
			if len(blob.Data) > 0 {
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
