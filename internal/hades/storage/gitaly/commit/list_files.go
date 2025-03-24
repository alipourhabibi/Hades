package commit

import (
	"context"
	"fmt"
	"io"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
)

func (c *CommitService) ListFiles(ctx context.Context, content *registryv1.UploadRequestContent) ([]string, error) {

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

func (c *CommitService) GetCommit(ctx context.Context, commit string) error {
	resp, err := c.client.FindCommit(ctx, &pb.FindCommitRequest{
		Revision: []byte(commit),
		Repository: &pb.Repository{
			StorageName:  c.defaultStorageName,
			RelativePath: "googleapis/googleapis",
			GlRepository: "googleapis/googleapis",
		},
	})
	if err != nil {
		return err
	}
	fmt.Println(string(resp.Commit.Body))
	return nil

	// moduleName := fmt.Sprintf("%s/%s", content.ModuleRef.Owner, content.ModuleRef.Module)
	// repo := &pb.Repository{
	// 	StorageName:  c.defaultStorageName,
	// 	RelativePath: moduleName,
	// 	GlRepository: moduleName,
	// }
	//
	// getFilesStream, err := c.client.ListFiles(ctx, &pb.ListFilesRequest{
	// 	Repository: repo,
	// })
	// if err != nil {
	// 	return nil, err
	// }
	// paths := []string{}
	// for {
	// 	files, err := getFilesStream.Recv()
	// 	if err != nil {
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		return nil, err
	// 	}
	// 	for _, p := range files.Paths {
	// 		paths = append(paths, string(p))
	// 	}
	// }
	// return paths, nil
}
