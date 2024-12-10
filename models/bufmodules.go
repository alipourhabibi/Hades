package models

import (
	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ModuleRef struct {
	Id     string `json:"id"`
	Owner  string `json:"owner,omitempty"`
	Module string `json:"module,omitempty"`
}

func FromBufModulePB(in *modulev1.Module) (*Module, error) {
	id, err := uuid.FromBytes([]byte(in.Id))
	if err != nil {
		return nil, err
	}
	ownerId, err := uuid.FromBytes([]byte(in.OwnerId))
	if err != nil {
		return nil, err
	}
	return &Module{
		ID:               id,
		CreateTime:       in.CreateTime.AsTime(),
		UpdateTime:       in.UpdateTime.AsTime(),
		Name:             in.Name,
		OwnerID:          ownerId,
		Visibility:       ModuleVisibility(in.Visibility),
		State:            ModuleState(in.State),
		Description:      in.Description,
		URL:              in.Url,
		DefaultLabelName: in.DefaultLabelName,
	}, nil
}

func ToBufModulePB(in *Module) *modulev1.Module {
	return &modulev1.Module{
		Id:               in.ID.String(),
		CreateTime:       timestamppb.New(in.CreateTime),
		UpdateTime:       timestamppb.New(in.UpdateTime),
		Name:             in.Name,
		OwnerId:          in.OwnerID.String(),
		Visibility:       modulev1.ModuleVisibility(in.Visibility),
		State:            modulev1.ModuleState(in.State),
		Description:      in.Description,
		Url:              in.URL,
		DefaultLabelName: in.DefaultLabelName,
	}
}
