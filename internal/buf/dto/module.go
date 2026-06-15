// Package dto provides conversion functions between internal protobuf
// types and the buf.build registry wire types.
package dto

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// ToBufModulePB converts an internal Module to the buf.build wire type.
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

// FromModuleRefPB converts a buf.build ModuleRef to the internal type.
func FromModuleRefPB(in *modulev1.ModuleRef) *registryv1.ModuleRef {
	if in == nil {
		return &registryv1.ModuleRef{}
	}
	if in.GetId() != "" {
		return &registryv1.ModuleRef{Id: in.GetId()}
	}
	return &registryv1.ModuleRef{
		Owner:  in.GetName().GetOwner(),
		Module: in.GetName().GetModule(),
	}
}

// FromResourceRefPB converts a buf.build ResourceRef to the internal ModuleRef type.
func FromResourceRefPB(in *modulev1.ResourceRef) *registryv1.ModuleRef {
	if in == nil {
		return &registryv1.ModuleRef{}
	}
	return &registryv1.ModuleRef{
		Id:     in.GetId(),
		Owner:  in.GetName().GetOwner(),
		Module: in.GetName().GetModule(),
	}
}
