package models

import (
	"time"

	"github.com/google/uuid"
)

type DigestType int32

const (
	DigestType_UNSPECIFIED DigestType = 0
	DigestType_B5          DigestType = 1
)

type Commit struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	CommitHash       string     `gorm:"type:varchar(40)"`
	CreateTime       time.Time  `gorm:"not null;index"`
	UpdateTime       time.Time  `gorm:"not null;index"`
	OwnerID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Owner            User       `gorm:"foreignKey:OwnerID"`
	ModuleID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	Module           Module     `gorm:"foreignKey:ModuleID"`
	DigestType       DigestType `gorm:"type:smallint;not null"`
	DigestValue      string     `gorm:"type:varchar(128);not null"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid;index"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID"`
	SourceControlURL string     `gorm:"type:text"`
}
