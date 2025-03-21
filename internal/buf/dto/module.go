package dto

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

func ToBufModulePB(in *registryv1.Module) *modulev1.Module {
	return &modulev1.Module{
		Id:               in.Id,
		CreateTime:       in.CreateTime,
		UpdateTime:       in.UpdateTime,
		Name:             in.Name,
		OwnerId:          in.OwnerId,
		Visibility:       modulev1.ModuleVisibility(in.Visibility),
		State:            modulev1.ModuleState(in.State),
		Description:      in.Description,
		Url:              in.Url,
		DefaultLabelName: in.DefaultLabelName,
	}
}
