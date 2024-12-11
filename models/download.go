package models

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
)

type DownloadResponseContent struct {
	Commit *Commit `json:"commit,omitempty"`
	Files  []*File `json:"files,omitempty"`
}

type DownloadResponse struct {
	Contents []*DownloadResponseContent `json:"contents,omitempty"`
}

func ToContentPB(in *DownloadResponseContent) *modulev1.DownloadResponse_Content {
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
