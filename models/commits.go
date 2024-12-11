package models

import (
	"errors"
	"time"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
	generalutils "github.com/alipourhabibi/Hades/utils/general"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type DigestType int32

const (
	DigestType_UNSPECIFIED DigestType = 0
	DigestType_B5          DigestType = 1
)

type Commit struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	CommitHash       string     `gorm:"not null;type:varchar(40)"` // TODO should be uniq?
	CreateTime       time.Time  `gorm:"not null" json:"create_time"`
	UpdateTime       time.Time  `gorm:"not null" json:"update_time"`
	OwnerID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Owner            User       `gorm:"foreignKey:OwnerID"`
	ModuleID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	Module           Module     `gorm:"foreignKey:ModuleID"`
	DigestType       DigestType `gorm:"type:smallint;not null"`
	DigestValue      string     `gorm:"type:varchar(128);not null"` // TODO should be uniq?
	CreatedByUserID  uuid.UUID  `gorm:"type:uuid;index"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID"`
	SourceControlURL string     `gorm:"type:text"`
}

func (s *Commit) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		return errors.New("CommitID is required")
	}

	s.CreateTime = time.Now()
	return
}

func FromCommitPB(in *modulev1.Commit) (*Commit, error) {
	id, err := uuid.Parse(in.Id)
	if err != nil {
		return nil, err
	}
	ownerId, err := uuid.Parse(in.OwnerId)
	if err != nil {
		return nil, err
	}
	moduleId, err := uuid.Parse(in.ModuleId)
	if err != nil {
		return nil, err
	}
	createByUserId, err := uuid.Parse(in.CreatedByUserId)
	if err != nil {
		return nil, err
	}
	return &Commit{
		ID:               id,
		CreateTime:       in.CreateTime.AsTime(),
		OwnerID:          ownerId,
		ModuleID:         moduleId,
		DigestType:       DigestType(in.Digest.Type),
		DigestValue:      string(in.Digest.Value),
		CreatedByUserID:  createByUserId,
		SourceControlURL: in.SourceControlUrl,
	}, nil
}

// FromString returns the uuid from the string.

func ToCommitPB(in *Commit) *modulev1.Commit {
	return &modulev1.Commit{
		Id:         generalutils.ToDashless(in.ID),
		CreateTime: timestamppb.New(in.CreateTime),
		OwnerId:    in.OwnerID.String(),
		ModuleId:   in.ModuleID.String(),
		Digest: &modulev1.Digest{
			Type:  modulev1.DigestType(in.DigestType),
			Value: []byte(in.DigestValue),
		},
		CreatedByUserId:  in.CreatedByUserID.String(),
		SourceControlUrl: in.SourceControlURL,
	}
}
