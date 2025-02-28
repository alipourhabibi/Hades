package commit

import (
	"context"

	"github.com/alipourhabibi/Hades/models"
)

func (c *CommitStorage) Create(ctx context.Context, commit *models.Commit) error {
	query := `
		INSERT INTO commits (
			id,
			commit_hash,
			create_time,
			update_time,
			owner_id,
			module_id,
			digest_type,
			digest_value,
			created_by_user_id,
			source_control_url
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10
		)
	`

	_, err := c.pool.Exec(ctx, query,
		commit.ID,
		commit.CommitHash,
		commit.CreateTime,
		commit.UpdateTime,
		commit.OwnerID,
		commit.ModuleID,
		commit.DigestType,
		commit.DigestValue,
		commit.CreatedByUserID,
		commit.SourceControlURL,
	)

	return err
}
