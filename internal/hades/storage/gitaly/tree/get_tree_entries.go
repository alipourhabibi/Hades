package tree

import (
	"context"
	"io"
	"path"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

// GetTreeEntries returns the depth-1 contents of the given directory inside
// the latest commit of the owner/module repository.
//
// dir is the path to list; an empty string or "." lists the repository root.
// The revision "HEAD" is used so callers always see the latest commit.
func (s *TreeService) GetTreeEntries(ctx context.Context, owner, module, dir string) ([]*registryv1.FileEntry, error) {
	repoPath := owner + "/" + module
	repo := &pb.Repository{
		StorageName:  s.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	// Normalise the path: empty or "." means root (".").
	if dir == "" || dir == "/" {
		dir = "."
	}
	dir = path.Clean(dir)

	stream, err := s.commitClient.GetTreeEntries(ctx, &pb.GetTreeEntriesRequest{
		Repository: repo,
		Revision:   []byte("HEAD"),
		Path:       []byte(dir),
		Recursive:  false,
	})
	if err != nil {
		return nil, err
	}

	var entries []*registryv1.FileEntry
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
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
			// Derive the base name from the full path.
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

	return entries, nil
}
