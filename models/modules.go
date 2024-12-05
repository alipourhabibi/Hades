package models

import (
	"time"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type ModuleVisibility int32

const (
	ModuleVisibility_MODULE_VISIBILITY_UNSPECIFIED ModuleVisibility = 0
	ModuleVisibility_MODULE_VISIBILITY_PUBLIC      ModuleVisibility = 1
	ModuleVisibility_MODULE_VISIBILITY_PRIVATE     ModuleVisibility = 2
)

type ModuleState int32

const (
	ModuleState_MODULE_STATE_UNSPECIFIED ModuleState = 0
	ModuleState_MODULE_STATE_ACTIVE      ModuleState = 1
	ModuleState_MODULE_STATE_DEPRECATED  ModuleState = 2
)

type Module struct {
	ID               uuid.UUID        `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreateTime       time.Time        `gorm:"not null" json:"create_time"`
	UpdateTime       time.Time        `gorm:"not null" json:"update_time"`
	Name             string           `gorm:"unique;not null" json:"name"`
	OwnerID          uuid.UUID        `gorm:"type:uuid;not null" json:"owner_id"`
	Owner            User             `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Visibility       ModuleVisibility `gorm:"not null;default:1" json:"visibility"`
	State            ModuleState      `gorm:"not null;default:1" json:"state"`
	Description      string           `gorm:"type:text" json:"description"`
	URL              string           `gorm:"index:url_unique,unique column:url" json:"url"`
	DefaultLabelName string           `gorm:"default:'main'" json:"default_label_name"`

	Commits []Commit `gorm:"foreignKey:ModuleID" json:"commits,omitempty"`
}

func (s *Module) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}

	s.CreateTime = time.Now()
	return
}

func FromModulePB(in *registrypbv1.Module) (*Module, error) {
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
		Description:      in.Description,
		DefaultLabelName: in.DefaultBranch,
	}, nil
}

func ToModulePB(in *Module) *registrypbv1.Module {
	return &registrypbv1.Module{
		Id:            in.ID.String(),
		CreateTime:    timestamppb.New(in.CreateTime),
		UpdateTime:    timestamppb.New(in.UpdateTime),
		Name:          in.Name,
		OwnerId:       in.OwnerID.String(),
		Visibility:    registrypbv1.EVisibility(in.Visibility),
		Description:   in.Description,
		DefaultBranch: in.DefaultLabelName,
	}
}
