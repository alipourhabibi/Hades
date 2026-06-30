package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CommitStorage handles commit CRUD against PostgreSQL.
type CommitStorage struct {
	pool *pgxpool.Pool
}

func NewCommit(pool *pgxpool.Pool) *CommitStorage {
	return &CommitStorage{pool: pool}
}

func (c *CommitStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return c.pool
}

func (c *CommitStorage) Create(
	ctx context.Context,
	id uuid.UUID,
	commitHash, ownerId, moduleId string,
	digestType registryv1.DigestType,
	digestValue, createdByUserId, sourceControlUrl string,
) error {
	query := `
INSERT INTO commits (
  id, commit_hash, owner_id, module_id,
  digest_type, digest_value, created_by_user_id, source_control_url
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := c.q(ctx).Exec(ctx, query,
		id, commitHash, ownerId, moduleId,
		digestType, digestValue, createdByUserId, sourceControlUrl,
	)
	return err
}

func (c *CommitStorage) GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error) {
	query := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  m.id, m.create_time, m.update_time, m.name, m.owner_id,
  m.visibility, m.description, m.default_branch, m.state, m.url, m.default_label_name
FROM commits c
LEFT JOIN modules m ON m.id = c.module_id
WHERE c.id = $1`

	cmt := &registryv1.Commit{
		Digest: &registryv1.Digest{},
		Module: &registryv1.Module{},
	}
	var createTime, updateTime time.Time
	var mCreateTime, mUpdateTime *time.Time

	err := c.q(ctx).QueryRow(ctx, query, id).Scan(
		&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
		&cmt.OwnerId, &cmt.ModuleId, &cmt.Digest.Type, &cmt.Digest.Value,
		&cmt.CreatedByUserId, &cmt.SourceControlUrl,
		&cmt.Module.Id, &mCreateTime, &mUpdateTime,
		&cmt.Module.Name, &cmt.Module.OwnerId,
		&cmt.Module.Visibility, &cmt.Module.Description,
		&cmt.Module.DefaultBranch, &cmt.Module.State,
		&cmt.Module.Url, &cmt.Module.DefaultLabelName,
	)
	if err != nil {
		return nil, err
	}
	cmt.CreateTime = timestamppb.New(createTime)
	cmt.UpdateTime = timestamppb.New(updateTime)
	if mCreateTime != nil {
		cmt.Module.CreateTime = timestamppb.New(*mCreateTime)
	}
	if mUpdateTime != nil {
		cmt.Module.UpdateTime = timestamppb.New(*mUpdateTime)
	}
	return cmt, nil
}

func (c *CommitStorage) GetCommitByQuery(ctx context.Context, queryMap map[string]any) (*registryv1.Commit, error) {
	baseQuery := `
SELECT id, commit_hash, create_time, update_time,
  owner_id, module_id, digest_type, digest_value,
  created_by_user_id, source_control_url
FROM commits`

	var conditions []string
	var values []interface{}
	i := 1
	for key, value := range queryMap {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", key, i))
		values = append(values, value)
		i++
	}
	if len(conditions) > 0 {
		baseQuery += " WHERE " + conditions[0]
		for _, cond := range conditions[1:] {
			baseQuery += " AND " + cond
		}
	}

	cmt := &registryv1.Commit{Digest: &registryv1.Digest{}}
	var createTime, updateTime time.Time
	err := c.q(ctx).QueryRow(ctx, baseQuery, values...).Scan(
		&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
		&cmt.OwnerId, &cmt.ModuleId,
		&cmt.Digest.Type, &cmt.Digest.Value,
		&cmt.CreatedByUserId, &cmt.SourceControlUrl,
	)
	cmt.CreateTime = timestamppb.New(createTime)
	cmt.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return cmt, nil
}

func (c *CommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	if len(moduleRefs) == 0 {
		return nil, nil
	}

	var idRefs []string
	var nameRefs []string
	for _, ref := range moduleRefs {
		if ref.Id != "" {
			idRefs = append(idRefs, ref.Id)
		} else {
			nameRefs = append(nameRefs, ref.Owner+"/"+ref.Module)
		}
	}

	var commits []*registryv1.Commit

	if len(idRefs) > 0 {
		q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username AS owner_name, u.id AS owner_id,
  m.name AS module_name, m.id AS module_id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.id = ANY($1::uuid[])`
		rows, err := c.q(ctx).Query(ctx, q, idRefs)
		if err != nil {
			return nil, err
		}
		result, err := scanCommitRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		commits = append(commits, result...)
	}

	if len(nameRefs) > 0 {
		q := `
SELECT DISTINCT ON (c.module_id)
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username AS owner_name, u.id AS owner_id,
  m.name AS module_name, m.id AS module_id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE m.name = ANY($1::text[])
ORDER BY c.module_id, c.create_time DESC`
		rows, err := c.q(ctx).Query(ctx, q, nameRefs)
		if err != nil {
			return nil, err
		}
		result, err := scanCommitRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		commits = append(commits, result...)
	}

	return commits, nil
}

func scanCommitRows(rows pgx.Rows) ([]*registryv1.Commit, error) {
	var commits []*registryv1.Commit
	for rows.Next() {
		cmt := &registryv1.Commit{Digest: &registryv1.Digest{}}
		var ownerName string
		var ownerID uuid.UUID
		var moduleName string
		var moduleID uuid.UUID
		var createTime, updateTime time.Time

		if err := rows.Scan(
			&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
			&cmt.OwnerId, &cmt.ModuleId,
			&cmt.Digest.Type, &cmt.Digest.Value,
			&cmt.CreatedByUserId, &cmt.SourceControlUrl,
			&ownerName, &ownerID, &moduleName, &moduleID,
		); err != nil {
			return nil, err
		}
		cmt.CreateTime = timestamppb.New(createTime)
		cmt.UpdateTime = timestamppb.New(updateTime)
		cmt.Owner = &registryv1.User{Username: ownerName}
		cmt.Module = &registryv1.Module{Name: moduleName}
		commits = append(commits, cmt)
	}
	return commits, rows.Err()
}

func (c *CommitStorage) ListByModule(ctx context.Context, moduleID string) ([]*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username, u.id, m.name, m.id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.module_id = $1
ORDER BY c.create_time DESC`

	rows, err := c.q(ctx).Query(ctx, q, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommitRows(rows)
}

func (c *CommitStorage) GetByHash(ctx context.Context, commitHash string) (*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username, u.id, m.name, m.id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.commit_hash = $1`

	rows, err := c.q(ctx).Query(ctx, q, commitHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commits, err := scanCommitRows(rows)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, pgx.ErrNoRows
	}
	return commits[0], nil
}

func (c *CommitStorage) GetByHashPrefix(ctx context.Context, prefix string) (*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username, u.id, m.name, m.id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.commit_hash ILIKE $1 || '%'
LIMIT 1`

	rows, err := c.q(ctx).Query(ctx, q, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commits, err := scanCommitRows(rows)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, pgx.ErrNoRows
	}
	return commits[0], nil
}

func (c *CommitStorage) DeleteByIds(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := c.q(ctx).Exec(ctx, `DELETE FROM commits WHERE id = ANY($1::uuid[])`, ids)
	return err
}

var _ commit.Storage = (*CommitStorage)(nil)
