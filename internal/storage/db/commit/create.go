package commit

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
)

func (c *CommitStorage) Create(ctx context.Context, commit *models.Commit) error {
	return c.db.Model(commit).Create(commit).Error
}
