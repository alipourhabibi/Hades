package tree

import (
	"context"
	"fmt"
	"io"
	"path"

	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

// GetFileContent fetches the raw byte content of a single file by its path
// inside the latest commit of the owner/module repository.
//
// It resolves the file OID via GetTreeEntries (scoped to the parent directory),
// then streams the blob content via GetBlob.
func (s *TreeService) GetFileContent(ctx context.Context, owner, module, filePath string) ([]byte, int64, error) {
	repoPath := owner + "/" + module
	repo := &pb.Repository{
		StorageName:  s.defaultStorageName,
		RelativePath: repoPath,
		GlRepository: repoPath,
	}

	// Resolve the OID of the file by listing its parent directory.
	dir := path.Dir(filePath)
	if dir == "." {
		dir = "."
	}
	base := path.Base(filePath)

	stream, err := s.commitClient.GetTreeEntries(ctx, &pb.GetTreeEntriesRequest{
		Repository: repo,
		Revision:   []byte("HEAD"),
		Path:       []byte(dir),
		Recursive:  false,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("GetTreeEntries: %w", err)
	}

	var oid string
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
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
		return nil, 0, fmt.Errorf("file not found: %s", filePath)
	}

	// Stream the blob content by OID.
	blobStream, err := s.blobClient.GetBlob(ctx, &pb.GetBlobRequest{
		Repository: repo,
		Oid:        oid,
		Limit:      -1, // full content
	})
	if err != nil {
		return nil, 0, fmt.Errorf("GetBlob: %w", err)
	}

	var content []byte
	var size int64
	for {
		msg, err := blobStream.Recv()
		if err == io.EOF {
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
