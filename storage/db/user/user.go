package user

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	"gorm.io/gorm"
)

type UserStorage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *UserStorage {
	return &UserStorage{
		db: db,
	}
}

func (u *UserStorage) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	user := &models.User{}
	err := u.db.Model(&models.User{}).First(user, "username = ?", username).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (u *UserStorage) Create(ctx context.Context, in *models.User) error {
	return u.db.Model(in).Create(in).Error
}

func (u *UserStorage) GetBySessionId(ctx context.Context, sessionId string) (*models.User, error) {
	var session models.Session

	if err := u.db.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Omit("password")
	}).First(&session, "id = ?", sessionId).Error; err != nil {
		return nil, err
	}

	// Return the associated user
	return &session.User, nil
}
