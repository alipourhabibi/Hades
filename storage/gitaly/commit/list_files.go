package commit

import (
	"context"
	"fmt"
	"io"

	"github.com/alipourhabibi/Hades/models"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

func (c *CommitService) ListFiles(ctx context.Context, content *models.UploadRequest_Content) ([]string, error) {

	moduleName := fmt.Sprintf("%s/%s", content.ModuleRef.Owner, content.ModuleRef.Module)
	repo := &pb.Repository{
		StorageName:  c.defaultStorageName,
		RelativePath: moduleName,
		GlRepository: moduleName,
	}

	getFilesStream, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{
		Repository: repo,
	})
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for {
		files, err := getFilesStream.Recv()
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
