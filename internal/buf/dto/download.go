package dto

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

func ToContentPB(in *registryv1.DownloadResponseContent) *modulev1.DownloadResponse_Content {
	files := []*modulev1.File{}
	for _, f := range in.Files {
		files = append(files, &modulev1.File{
			Path:    f.Path,
			Content: f.Content,
		})
	}
	return &modulev1.DownloadResponse_Content{
		Commit: ToCommitPB(in.Commit),
		Files:  files,
	}
}
