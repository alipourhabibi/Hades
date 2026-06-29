package gitaly

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RepositoryService wraps the Gitaly RepositoryService gRPC client.
type RepositoryService struct {
	client             pb.RepositoryServiceClient
	defaultStorageName string
}

func newRepositoryService(c config.Gitaly) (*RepositoryService, error) {
	conn, err := grpc.NewClient(gitalyAddr(c), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &RepositoryService{
		client:             pb.NewRepositoryServiceClient(conn),
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// DeleteRepository removes the Gitaly repository for the given module.
func (c *RepositoryService) DeleteRepository(ctx context.Context, in *registryv1.Module) error {
	_, err := c.client.RemoveRepository(ctx, &pb.RemoveRepositoryRequest{
		Repository: &pb.Repository{
			RelativePath: in.Name,
			StorageName:  c.defaultStorageName,
			GlRepository: in.Name,
		},
	})
	return err
}

// CreateRepository initialises a new bare Gitaly repository for the module.
func (c *RepositoryService) CreateRepository(ctx context.Context, in *registryv1.Module) error {
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	_, err := c.client.CreateRepository(ctx, &pb.CreateRepositoryRequest{
		ObjectFormat:  pb.ObjectFormat_OBJECT_FORMAT_SHA1,
		DefaultBranch: []byte(in.DefaultBranch),
		Repository: &pb.Repository{
			RelativePath: in.Name,
			StorageName:  c.defaultStorageName,
			GlRepository: in.Name,
		},
	})
	return err
}
