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

func (m *ModuleStorage) Create(ctx context.Context, in *models.Module) (*models.Module, error) {
	err := m.db.Model(in).Create(in).Error
	return in, err
}

func (m *ModuleStorage) GetModulesByRefs(ctx context.Context, refs ...*models.ModuleRef) ([]*models.Module, error) {
	query := m.db.
		Model(&models.Module{})

	for _, req := range refs {
		if req.Id != "" {
			query.Or("id = ?", req.Id)
		} else {
			query.Or("modules.name", req.Owner+"/"+req.Module)
		}
	}

	modules := []*models.Module{}

	err := query.Find(&modules).Error
	if err != nil {
		return nil, err
	}

	return modules, nil
}
