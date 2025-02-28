package commit

import (
	"context"
	"errors"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CommitStorage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *CommitStorage {
	return &CommitStorage{
		pool: pool,
	}
}

func (c *CommitStorage) GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error) {
	query := `
SELECT 
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
FROM commits
WHERE id = $1
	`

	commit := &registryv1.Commit{
		Digest: &registryv1.Digest{},
	}
	var createTime time.Time
	var updateTime time.Time
	err := c.pool.QueryRow(ctx, query, id).Scan(
		&commit.Id,
		&commit.CommitHash,
		&createTime,
		&updateTime,
		&commit.OwnerId,
		&commit.ModuleId,
		&commit.Digest.Type,
		&commit.Digest.Value,
		&commit.CreatedByUserId,
		&commit.SourceControlUrl,
	)
	commit.CreateTime = timestamppb.New(createTime)
	commit.UpdateTime = timestamppb.New(updateTime)

	if err != nil {
		return nil, err
	}
	return commit, nil
}

func (c *CommitStorage) GetCommitByQuery(ctx context.Context, query map[string]any) (*registryv1.Commit, error) {
	baseQuery := `
SELECT 
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
FROM commits
	`

	var conditions []string
	var values []interface{}
	i := 1

	for key, value := range query {
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

	commit := &registryv1.Commit{
		Digest: &registryv1.Digest{},
	}
	var createTime time.Time
	var updateTime time.Time
	err := c.pool.QueryRow(ctx, baseQuery, values...).Scan(
		&commit.Id,
		&commit.CommitHash,
		&createTime,
		&updateTime,
		&commit.OwnerId,
		&commit.ModuleId,
		&commit.Digest.Type,
		&commit.Digest.Value,
		&commit.CreatedByUserId,
		&commit.SourceControlUrl,
	)
	commit.CreateTime = timestamppb.New(createTime)
	commit.UpdateTime = timestamppb.New(updateTime)

	if err != nil {
		if errors.Is(pgx.ErrNoRows, err) {
			return nil, nil
		}
		return nil, err
	}
	return commit, nil
}

// TODO think about it
func (c *CommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	baseQuery := `
SELECT 
  c.id,
  c.commit_hash,
  c.create_time,
  c.update_time,
  c.owner_id,
  c.module_id,
  c.digest_type,
  c.digest_value,
  c.created_by_user_id,
  c.source_control_url,
  u.username AS owner_name,
  u.id AS owner_id,
  m.name AS module_name,
  m.id AS module_id
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id`

	var commits []*registryv1.Commit

	for _, req := range moduleRefs {
		var (
			query  string
			values []interface{}
		)

		if req.Id != "" {
			query = baseQuery + " WHERE c.id = $1"
			values = append(values, req.Id)
		} else {
			query = baseQuery + " WHERE m.name = $1"
			values = append(values, req.Owner+"/"+req.Module)
		}
		query += " ORDER BY c.create_time DESC LIMIT 1"

		rows, err := c.pool.Query(ctx, query, values...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			commit := &registryv1.Commit{
				Digest: &registryv1.Digest{},
			}
			var ownerName string
			var ownerId uuid.UUID
			var moduleName string
			var moduleId uuid.UUID

			var createTime time.Time
			var updateTime time.Time

			err := rows.Scan(
				&commit.Id,
				&commit.CommitHash,
				&createTime,
				&updateTime,
				&commit.OwnerId,
				&commit.ModuleId,
				&commit.Digest.Type,
				&commit.Digest.Value,
				&commit.CreatedByUserId,
				&commit.SourceControlUrl,
				&ownerName,
				&ownerId,
				&moduleName,
				&moduleId,
			)

			commit.CreateTime = timestamppb.New(createTime)
			commit.UpdateTime = timestamppb.New(updateTime)
			if err != nil {
				return nil, err
			}

			commit.Owner = &registryv1.User{
				Username: ownerName,
			}
			commit.Module = &registryv1.Module{
				Name: moduleName,
			}

			commits = append(commits, commit)
		}

		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return commits, nil
}
