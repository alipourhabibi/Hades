// Package commit provides PostgreSQL storage for proto module commits,
// tracking content digests and ownership so the registry can resolve
// references without hitting Gitaly.
package commit

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
)

// Create inserts a commit row linking a Gitaly commit hash to its module,
// owner, and content digest. The caller is responsible for ensuring the
// Gitaly commit already exists before calling this.
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

	_, err := c.db.Exec(ctx, query,
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
