// Package repository wraps the Gitaly RepositoryService gRPC client.
package repository

import (
	"context"
	"fmt"

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

// NewDefault dials the Gitaly server and returns a RepositoryService.
func NewDefault(c config.Gitaly) (*RepositoryService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", func() string { if c.Host != "" { return c.Host }; return "localhost" }(), c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewRepositoryServiceClient(conn)
	return &RepositoryService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// DeleteRepository removes the Gitaly repository for the given module.
// Used as a compensating action when a DB transaction fails after the
// repository was already created (saga pattern).
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
	if err != nil {
		return err
	}

	return nil

}
