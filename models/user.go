// models/user.go
package models

import (
	"time"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type UserType int32

const (
	UserType_USER_TYPE_UNSPECIFIED  UserType = 0
	UserType_USER_TYPE_ORGANIZATION UserType = 1
	UserType_USER_TYPE_USER         UserType = 2
)

type UserState int32

const (
	UserState_USER_STATE_UNSPECIFIED UserState = 0
	UserState_USER_STATE_ACTIVE      UserState = 1
	UserState_USER_STATE_DEACTIVATED UserState = 2
)

type User struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CreateTime  time.Time `gorm:"not null" json:"create_time"`
	UpdateTime  time.Time `gorm:"not null" json:"update_time"`
	Username    string    `gorm:"uniqueIndex;not null" json:"username"`
	Email       string    `gorm:"uniqueIndex;not null" json:"email"`
	Password    string    `gorm:"column:password;type:varchar(255);not null" json:"password" binding:"required"`
	Type        UserType  `gorm:"not null;default:0" json:"type"`
	State       UserState `gorm:"not null;default:1" json:"state"`
	Description string    `gorm:"type:text" json:"description"`
	URL         string    `gorm:"column:url" json:"url"`
}

func FromUserRegistryPbV1(in *v1.User) (*User, error) {
	id, err := uuid.FromBytes([]byte(in.Id))
	if err != nil {
		return nil, err
	}
	return &User{
		ID:          id,
		CreateTime:  in.CreateTime.AsTime(),
		UpdateTime:  in.UpdateTime.AsTime(),
		Username:    in.Username,
		Description: in.Description,
	}, nil
}

func ToUserRegistryPbV1(in *User) (*v1.User, error) {
	return &v1.User{
		Id:          in.ID.String(),
		CreateTime:  timestamppb.New(in.CreateTime),
		UpdateTime:  timestamppb.New(in.UpdateTime),
		Username:    in.Username,
		Description: in.Description,
	}, nil
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}

	u.CreateTime = time.Now()
	return
}
