package session

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	"gorm.io/gorm"
)

type SessionStorage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *SessionStorage {
	return &SessionStorage{
		db: db,
	}
}

func (s *SessionStorage) Create(ctx context.Context, session *models.Session) error {
	return s.db.Model(&models.Session{}).Create(session).Error
}

func (s *SessionStorage) GetById(ctx context.Context, id string) (*models.Session, error) {
	session := &models.Session{}
	tx := s.db.Model(&models.Session{}).Find(session, "id = ?", id)
	if err := tx.Error; err != nil {
		return nil, err
	}
	return session, nil
}
