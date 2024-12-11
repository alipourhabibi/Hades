package repository

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/models"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RepositoryService struct {
	client             pb.RepositoryServiceClient
	defaultStorageName string
}

func NewDefault(c config.Gitaly) (*RepositoryService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf(":%d", c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewRepositoryServiceClient(conn)
	return &RepositoryService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

func (c *RepositoryService) CreateRepository(ctx context.Context, in *models.Module) error {
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
	if err != nil {
		return err
	}

	return nil

}
