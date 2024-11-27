package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Session struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreateTime time.Time `gorm:"not null" json:"create_time"`
	UpdateTime time.Time `gorm:"not null" json:"update_time"`
	UserID     uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User       User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	AuthModule string    `gorm:"type:varchar(100);not null" json:"auth_module"`
	ExpiresAt  time.Time `grom:"not null" json:"expires_at"`
}

func (u *Session) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}

	u.CreateTime = time.Now()
	return
}
