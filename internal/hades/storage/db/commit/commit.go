// Package commit provides PostgreSQL storage for commit metadata.
package commit

import (
	"context"
	"errors"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// CommitStorage handles commit CRUD against PostgreSQL.
type CommitStorage struct {
	db querier
}

// New returns a CommitStorage backed by the given pool.
func New(pool *pgxpool.Pool) *CommitStorage {
	return &CommitStorage{
		db: pool,
	}
}

// WithTx returns a copy of CommitStorage bound to the given transaction.
func (c *CommitStorage) WithTx(tx pgx.Tx) *CommitStorage {
	return &CommitStorage{db: tx}
}

// GetCommitById returns a commit with its associated module by primary key.
func (c *CommitStorage) GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error) {
	query := `
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

  m.id,
  m.create_time,
  m.update_time,
  m.name,
  m.owner_id,
  m.visibility,
  m.description,
  m.default_branch,
  m.state,
  m.url,
  m.default_label_name
FROM commits c
LEFT JOIN modules m ON m.id = c.module_id
WHERE c.id = $1
	`

	commit := &registryv1.Commit{
		Digest: &registryv1.Digest{},
		Module: &registryv1.Module{},
	}

	var createTime, updateTime time.Time
	var mCreateTime, mUpdateTime *time.Time

	err := c.db.QueryRow(ctx, query, id).Scan(
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

		&commit.Module.Id,
		&mCreateTime,
		&mUpdateTime,
		&commit.Module.Name,
		&commit.Module.OwnerId,
		&commit.Module.Visibility,
		&commit.Module.Description,
		&commit.Module.DefaultBranch,
		&commit.Module.State,
		&commit.Module.Url,
		&commit.Module.DefaultLabelName,
	)
	if err != nil {
		return nil, err
	}

	commit.CreateTime = timestamppb.New(createTime)
	commit.UpdateTime = timestamppb.New(updateTime)

	if mCreateTime != nil {
		commit.Module.CreateTime = timestamppb.New(*mCreateTime)
	}
	if mUpdateTime != nil {
		commit.Module.UpdateTime = timestamppb.New(*mUpdateTime)
	}

	return commit, nil
}

// GetCommitByQuery returns a single commit matching the given column/value pairs.
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
	err := c.db.QueryRow(ctx, baseQuery, values...).Scan(
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return commit, nil
}

// GetCommitByOwnerModule resolves module refs to commits. Refs with an ID
// are looked up directly; refs with owner/module are resolved to the latest
// commit per module.
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

	// Resolve by commit ID.
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
		rows, err := c.db.Query(ctx, q, idRefs)
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

	// Resolve by module name - returns the latest commit per module.
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
		rows, err := c.db.Query(ctx, q, nameRefs)
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
		commit := &registryv1.Commit{Digest: &registryv1.Digest{}}
		var ownerName string
		var ownerID uuid.UUID
		var moduleName string
		var moduleID uuid.UUID
		var createTime, updateTime time.Time

		if err := rows.Scan(
			&commit.Id, &commit.CommitHash, &createTime, &updateTime,
			&commit.OwnerId, &commit.ModuleId,
			&commit.Digest.Type, &commit.Digest.Value,
			&commit.CreatedByUserId, &commit.SourceControlUrl,
			&ownerName, &ownerID, &moduleName, &moduleID,
		); err != nil {
			return nil, err
		}
		commit.CreateTime = timestamppb.New(createTime)
		commit.UpdateTime = timestamppb.New(updateTime)
		commit.Owner = &registryv1.User{Username: ownerName}
		commit.Module = &registryv1.Module{Name: moduleName}
		commits = append(commits, commit)
	}
	return commits, rows.Err()
}

// ListByModule returns all commits for the given moduleID ordered newest first.
// The returned commits include a partial Owner (username only) and Module (name
// only) from the JOIN; full user/module details are not hydrated.
func (c *CommitStorage) ListByModule(ctx context.Context, moduleID string) ([]*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username AS owner_name, u.id AS owner_id_ref,
  m.name AS module_name, m.id AS module_id_ref
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.module_id = $1
ORDER BY c.create_time DESC`

	rows, err := c.db.Query(ctx, q, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommitRows(rows)
}

// GetByHash returns the commit whose commit_hash equals the given value.
// Returns pgx.ErrNoRows if no such commit exists.
func (c *CommitStorage) GetByHash(ctx context.Context, commitHash string) (*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username AS owner_name, u.id AS owner_id_ref,
  m.name AS module_name, m.id AS module_id_ref
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.commit_hash = $1`

	rows, err := c.db.Query(ctx, q, commitHash)
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

// GetByHashPrefix returns the commit whose commit_hash starts with prefix
// (case-insensitive). Returns pgx.ErrNoRows if no match is found.
func (c *CommitStorage) GetByHashPrefix(ctx context.Context, prefix string) (*registryv1.Commit, error) {
	q := `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username AS owner_name, u.id AS owner_id_ref,
  m.name AS module_name, m.id AS module_id_ref
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id
WHERE c.commit_hash ILIKE $1 || '%'
LIMIT 1`

	rows, err := c.db.Query(ctx, q, prefix)
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

// DeleteByIds deletes commits by their IDs.
func (c *CommitStorage) DeleteByIds(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := c.db.Exec(ctx, `DELETE FROM commits WHERE id = ANY($1::uuid[])`, ids)
	return err
}
