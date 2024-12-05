package module

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
	"gorm.io/gorm"
)

type ModuleStorage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *ModuleStorage {
	return &ModuleStorage{
		db: db,
	}
}

func (m *ModuleStorage) Create(ctx context.Context, in *models.Module) error {
	return m.db.Model(in).Create(in).Error
}
