package gitaly

import (
	"context"
	"fmt"
	"io"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// CommitService wraps the Gitaly CommitService gRPC client.
type CommitService struct {
	client             pb.CommitServiceClient
	defaultStorageName string
}

func newCommitService(c config.Gitaly) (*CommitService, error) {
	conn, err := grpc.NewClient(gitalyAddr(c), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &CommitService{
		client:             pb.NewCommitServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// ListFiles returns the paths of all files at HEAD of the module referenced
// by the upload request.
func (c *CommitService) ListFiles(ctx context.Context, content *registryv1.UploadRequestContent) ([]string, error) {
	moduleName := fmt.Sprintf("%s/%s", content.ModuleRef.Owner, content.ModuleRef.Module)
	repo := &pb.Repository{
		StorageName:  c.defaultStorageName,
		RelativePath: moduleName,
		GlRepository: moduleName,
	}

	stream, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{Repository: repo})
	if err != nil {
		return nil, err
	}
	var paths []string
	for {
		files, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		for _, p := range files.Paths {
			paths = append(paths, string(p))
		}
	}
	return paths, nil
}
