package dto

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// FromUploadContentPB converts a single buf.build upload content item to the internal type.
func FromUploadContentPB(in *modulev1.UploadRequest_Content) *registryv1.UploadRequestContent {
	if in == nil {
		return nil
	}
	content := &registryv1.UploadRequestContent{
		ModuleRef: &registryv1.ModuleRef{
			Id:     in.ModuleRef.GetId(),
			Owner:  in.ModuleRef.GetName().GetOwner(),
			Module: in.ModuleRef.GetName().GetModule(),
		},
		Files: make([]*registryv1.File, 0, len(in.Files)),
	}
	for _, f := range in.Files {
		content.Files = append(content.Files, &registryv1.File{
			Path:    f.Path,
			Content: f.Content,
		})
	}
	return content
}
