package commit

import (
	"context"
	"fmt"

	"github.com/alipourhabibi/Hades/models"
	"gorm.io/gorm"
)

type CommitStorage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *CommitStorage {
	return &CommitStorage{
		db: db,
	}
}

func (c *CommitStorage) GetCommitById(ctx context.Context, id string) (*models.Commit, error) {
	commit := &models.Commit{}
	tx := c.db.Model(&models.Commit{}).Find(commit, "id = ?", id)
	if err := tx.Error; err != nil {
		return nil, err
	}
	return commit, nil
}

func (c *CommitStorage) GetCommitByQuery(ctx context.Context, query map[string]any) (*models.Commit, error) {
	commit := &models.Commit{}
	q := c.db.Model(commit)

	// Build the query dynamically
	for key, value := range query {
		q = q.Where(fmt.Sprintf("%s = ?", key), value)
	}

	err := q.First(commit).Error
	return commit, err
}

func (c *CommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*models.ModuleRef) ([]*models.Commit, error) {

	query := c.db.
		Joins("JOIN modules ON commits.module_id = modules.id").
		Preload("Module").
		Model(&models.Commit{})

	for _, req := range moduleRefs {
		if req.Id != "" {
			query.Or("id = ?", req.Id)
		} else {
			query.Or("modules.name", req.Owner+"/"+req.Module)
		}
	}

	commits := []*models.Commit{}

	return commits, nil
}
