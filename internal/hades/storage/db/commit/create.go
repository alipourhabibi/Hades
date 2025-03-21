package commit

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
)

func (c *CommitStorage) Create(
	ctx context.Context,
	id uuid.UUID,
	commitHash string,
	ownerId string,
	moduleId string,
	digestType registryv1.DigestType,
	digestValue string,
	createdByUserId string,
	sourceControlUrl string,
) error {
	query := `
INSERT INTO commits (
  id,
  commit_hash,
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
  $8
)`

	_, err := c.pool.Exec(ctx, query,
		id,
		commitHash,
		ownerId,
		moduleId,
		digestType,
		digestValue,
		createdByUserId,
		sourceControlUrl,
	)

	return err
}
