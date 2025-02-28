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

// TODO think about it
func (c *CommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*models.ModuleRef) ([]*models.Commit, error) {

	commits := []*models.Commit{}

	query := c.db.Order("create_time DESC").
		Joins("JOIN modules ON commits.module_id = modules.id").
		Preload("Module").
		Model(&models.Commit{})

	for _, req := range moduleRefs {
		commit := &models.Commit{}
		var err error
		if req.Id != "" {
			err = query.Where("commits.id = ?", req.Id).First(commit).Error
		} else {
			err = query.Where("modules.name", req.Owner+"/"+req.Module).First(commit).Error
		}
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}

	return commits, nil
}
