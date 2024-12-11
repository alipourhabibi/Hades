package blob

import (
	"context"
	"fmt"
	"io"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/models"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BlobService struct {
	client             pb.BlobServiceClient
	defaultStorageName string
}

func NewDefault(c config.Gitaly) (*BlobService, error) {
	conn, err := grpc.NewClient(fmt.Sprintf(":%d", c.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := pb.NewBlobServiceClient(conn)
	return &BlobService{
		client:             client,
		defaultStorageName: c.DefaultStorageName,
	}, nil
}

// ListBlobs returns a list of blobs based on the given commit
func (b *BlobService) ListBlobs(ctx context.Context, commit *models.Commit) ([]*models.DownloadResponseContent, error) {
	moduleName := fmt.Sprintf("%s", commit.Module.Name)
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

	// TODO make it better
	contents := []*models.DownloadResponseContent{}
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error receiving blob: %w", err)
		}

		if msg.Blobs != nil {
			tmp := &models.DownloadResponseContent{
				Commit: commit,
			}
			mapFiles := map[string]*models.File{}
			for _, blob := range msg.Blobs {
				if blob.Size != 0 {
					mapFiles[string(blob.GetPath())] = &models.File{
						Path:    string(blob.GetPath()),
						Content: blob.Data,
					}
				} else {
					mapFiles[string(blob.GetPath())].Content = append(mapFiles[string(blob.GetPath())].Content, blob.Data...)
				}
			}
			files := []*models.File{}
			for _, f := range mapFiles {
				files = append(files, f)
			}
			tmp.Files = files
			contents = append(contents, tmp)
		}
	}

	return contents, nil
}
