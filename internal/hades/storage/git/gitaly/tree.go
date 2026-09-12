package gitaly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
)

// ErrTreeNotFound is returned when a requested path or revision is absent from
// the repository. GitalyStorage maps it onto git.ErrNotFound, which is the
// error the git.Storage interface documents.
var ErrTreeNotFound = errors.New("gitaly: tree entry not found")

// TreeService wraps Gitaly's CommitServiceClient and BlobServiceClient.
type TreeService struct {
	commitClient       pb.CommitServiceClient
	blobClient         pb.BlobServiceClient
	defaultStorageName string
}

func newTreeService(conn *grpc.ClientConn, c config.Gitaly) *TreeService {
	return &TreeService{
		commitClient:       pb.NewCommitServiceClient(conn),
		blobClient:         pb.NewBlobServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}
}

// revisionOrHead returns the git revision to read, defaulting to HEAD.
//
// Callers pass a commit hash when the request named one. Reading HEAD in that
// case would silently answer from the default branch, so the caller would get
// content from a commit other than the one it asked for.
func revisionOrHead(ref string) []byte {
	if ref == "" {
		return []byte("HEAD")
	}
	return []byte(ref)
}

// GetFileContent fetches the raw byte content of a single file at ref.
// An empty ref reads the repository HEAD.
func (s *TreeService) GetFileContent(ctx context.Context, owner, module, ref, filePath string) ([]byte, int64, error) {
	repoPath := owner + "/" + module
	repo := &pb.Repository{
		StorageName:  s.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	dir := path.Dir(filePath)
	if dir == "." {
		dir = "."
	}
	base := path.Base(filePath)

	stream, err := s.commitClient.GetTreeEntries(ctx, &pb.GetTreeEntriesRequest{
		Repository: repo,
		Revision:   revisionOrHead(ref),
		Path:       []byte(dir),
		Recursive:  false,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("GetTreeEntries: %w", err)
	}

	var oid string
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("GetTreeEntries recv: %w", err)
		}
		for _, e := range resp.Entries {
			if path.Base(string(e.Path)) == base {
				oid = e.Oid
				break
			}
		}
		if oid != "" {
			break
		}
	}

	if oid == "" {
		return nil, 0, ErrTreeNotFound
	}

	blobStream, err := s.blobClient.GetBlob(ctx, &pb.GetBlobRequest{
		Repository: repo,
		Oid:        oid,
		Limit:      -1,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("GetBlob: %w", err)
	}

	var content []byte
	var size int64
	for {
		msg, err := blobStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("GetBlob recv: %w", err)
		}
		if msg.Size != 0 {
			size = msg.Size
		}
		content = append(content, msg.Data...)
	}

	return content, size, nil
}

// GetTreeEntries returns the depth-1 contents of dir at ref.
// An empty ref reads the repository HEAD.
func (s *TreeService) GetTreeEntries(ctx context.Context, owner, module, ref, dir string) ([]*registryv1.FileEntry, error) {
	repoPath := owner + "/" + module
	repo := &pb.Repository{
		StorageName:  s.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	if dir == "" || dir == "/" {
		dir = "."
	}
	dir = path.Clean(dir)

	stream, err := s.commitClient.GetTreeEntries(ctx, &pb.GetTreeEntriesRequest{
		Repository: repo,
		Revision:   revisionOrHead(ref),
		Path:       []byte(dir),
		Recursive:  false,
	})
	if err != nil {
		return nil, err
	}

	var entries []*registryv1.FileEntry
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, e := range resp.Entries {
			entry := &registryv1.FileEntry{
				Oid:  e.Oid,
				Mode: e.Mode,
				Path: string(e.Path),
			}
			entry.Name = path.Base(entry.Path)
			switch e.Type {
			case pb.TreeEntry_TREE:
				entry.Type = registryv1.FileEntryType_FILE_ENTRY_TYPE_DIR
			default:
				entry.Type = registryv1.FileEntryType_FILE_ENTRY_TYPE_FILE
			}
			entries = append(entries, entry)
		}
	}

	// Git cannot store an empty directory, so no entries under a named path
	// means the path is not in the tree. Reporting that as an empty listing
	// would make a typo look like an empty package.
	if len(entries) == 0 && dir != "." {
		return nil, ErrTreeNotFound
	}

	return entries, nil
}
