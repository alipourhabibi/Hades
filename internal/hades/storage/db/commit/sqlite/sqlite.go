package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteCommitStorage implements commit.Storage using database/sql with SQLite.
type SQLiteCommitStorage struct {
	db  *sql.DB
	res resource.Storage
}

func NewCommit(db *sql.DB, res resource.Storage) *SQLiteCommitStorage {
	return &SQLiteCommitStorage{db: db, res: res}
}

func (c *SQLiteCommitStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return c.db
}

func (c *SQLiteCommitStorage) Create(ctx context.Context, id uuid.UUID, commitHash, ownerId, moduleId string, digestType registryv1.DigestType, digestValue, createdByUserId, sourceControlUrl string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	ownerId = sqlutil.ID(ownerId)
	moduleId = sqlutil.ID(moduleId)
	createdByUserId = sqlutil.ID(createdByUserId)
	_, err := c.q(ctx).ExecContext(ctx, `
INSERT INTO commits (id, commit_hash, owner_id, module_id, digest_type, digest_value, created_by_user_id, source_control_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sqlutil.UUID(id), commitHash, ownerId, moduleId,
		digestType, digestValue, createdByUserId, sourceControlUrl)
	if err != nil {
		return err
	}
	return c.res.Register(ctx, sqlutil.UUID(id), resource.ResourceTypeCommit)
}

func scanSQLiteCommitRows(rows *sql.Rows) ([]*registryv1.Commit, error) {
	var commits []*registryv1.Commit
	for rows.Next() {
		cmt := &registryv1.Commit{Digest: &registryv1.Digest{}}
		var ownerName, moduleName string
		var createTime, updateTime sqltypes.Time
		// digest_value is hex text in the column and raw bytes on the wire.
		var digestHex *string
		if err := rows.Scan(
			&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
			&cmt.OwnerId, &cmt.ModuleId,
			&cmt.Digest.Type, &digestHex,
			&cmt.CreatedByUserId, &cmt.SourceControlUrl,
			&ownerName, &moduleName,
		); err != nil {
			return nil, err
		}
		cmt.Digest.Value = commit.DecodeDigest(digestHex)
		cmt.Id = sqlutil.Canonical(cmt.Id)
		cmt.OwnerId = sqlutil.Canonical(cmt.OwnerId)
		cmt.ModuleId = sqlutil.Canonical(cmt.ModuleId)
		cmt.CreatedByUserId = sqlutil.Canonical(cmt.CreatedByUserId)
		cmt.CreateTime = timestamppb.New(createTime.V)
		cmt.UpdateTime = timestamppb.New(updateTime.V)
		cmt.Owner = &identityv1.User{Username: ownerName}
		cmt.Module = &registryv1.Module{Name: moduleName}
		commits = append(commits, cmt)
	}
	return commits, rows.Err()
}

const sqliteCommitJoinCols = `
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  u.username, m.name
FROM commits c
JOIN users u ON u.id = c.owner_id
JOIN modules m ON m.id = c.module_id`

func (c *SQLiteCommitStorage) GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	id = sqlutil.ID(id)
	cmt := &registryv1.Commit{Digest: &registryv1.Digest{}, Module: &registryv1.Module{}}
	var createTime, updateTime sqltypes.Time
	var mCreateTime, mUpdateTime sqltypes.NullTime
	// digest_value is hex text in the column and raw bytes on the wire.
	var digestHex *string

	err := c.q(ctx).QueryRowContext(ctx, `
SELECT
  c.id, c.commit_hash, c.create_time, c.update_time,
  c.owner_id, c.module_id, c.digest_type, c.digest_value,
  c.created_by_user_id, c.source_control_url,
  m.id, m.create_time, m.update_time, m.name, m.owner_id,
  m.visibility, m.description, m.default_branch, m.state, m.url, m.default_label_name
FROM commits c
LEFT JOIN modules m ON m.id = c.module_id
WHERE c.id = ?`, id).Scan(
		&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
		&cmt.OwnerId, &cmt.ModuleId, &cmt.Digest.Type, &digestHex,
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
	cmt.Digest.Value = commit.DecodeDigest(digestHex)
	cmt.Id = sqlutil.Canonical(cmt.Id)
	cmt.OwnerId = sqlutil.Canonical(cmt.OwnerId)
	cmt.ModuleId = sqlutil.Canonical(cmt.ModuleId)
	cmt.CreatedByUserId = sqlutil.Canonical(cmt.CreatedByUserId)
	cmt.Module.Id = sqlutil.Canonical(cmt.Module.Id)
	cmt.Module.OwnerId = sqlutil.Canonical(cmt.Module.OwnerId)
	cmt.CreateTime = timestamppb.New(createTime.V)
	cmt.UpdateTime = timestamppb.New(updateTime.V)
	if mCreateTime.Valid {
		cmt.Module.CreateTime = timestamppb.New(mCreateTime.Time)
	}
	if mUpdateTime.Valid {
		cmt.Module.UpdateTime = timestamppb.New(mUpdateTime.Time)
	}
	return cmt, nil
}

func (c *SQLiteCommitStorage) GetCommitByDigest(ctx context.Context, moduleID, digestValue string) (*registryv1.Commit, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	moduleID = sqlutil.ID(moduleID)
	q := `SELECT id, commit_hash, create_time, update_time, owner_id, module_id, digest_type, digest_value, created_by_user_id, source_control_url FROM commits WHERE module_id = ? AND digest_value = ? LIMIT 1`
	cmt := &registryv1.Commit{Digest: &registryv1.Digest{}}
	var createTime, updateTime sqltypes.Time
	var digestHex *string
	err := c.q(ctx).QueryRowContext(ctx, q, moduleID, digestValue).Scan(
		&cmt.Id, &cmt.CommitHash, &createTime, &updateTime,
		&cmt.OwnerId, &cmt.ModuleId,
		&cmt.Digest.Type, &digestHex,
		&cmt.CreatedByUserId, &cmt.SourceControlUrl,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	cmt.Digest.Value = commit.DecodeDigest(digestHex)
	cmt.Id = sqlutil.Canonical(cmt.Id)
	cmt.OwnerId = sqlutil.Canonical(cmt.OwnerId)
	cmt.ModuleId = sqlutil.Canonical(cmt.ModuleId)
	cmt.CreatedByUserId = sqlutil.Canonical(cmt.CreatedByUserId)
	cmt.CreateTime = timestamppb.New(createTime.V)
	cmt.UpdateTime = timestamppb.New(updateTime.V)
	return cmt, nil
}

func (c *SQLiteCommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error) {
	if len(moduleRefs) == 0 {
		return nil, nil
	}
	var commits []*registryv1.Commit
	for _, ref := range moduleRefs {
		var rows *sql.Rows
		var err error
		if ref.Id != "" {
			rows, err = c.q(ctx).QueryContext(ctx,
				`SELECT `+sqliteCommitJoinCols+` WHERE c.id = ?`, sqlutil.ID(ref.Id))
		} else {
			// Latest commit per module (SQLite DISTINCT ON equivalent via subquery)
			//
			// rowid is the tiebreaker, and it is load-bearing. create_time
			// defaults to datetime('now'), which has one-second resolution, so
			// a module created and pushed to within the same second has two
			// rows carrying the identical timestamp. With no second key SQLite
			// may return either, and it returned the initial README-only
			// commit: every module created by a script or by the UI served an
			// empty module to buf, forever, until a later push happened to land
			// in a different second. rowid is monotonic in insertion order, so
			// it means "the one written last", which is what "latest" is asking
			// for. This table is a rowid table; keep it that way or replace
			// this with an explicit sequence.
			rows, err = c.q(ctx).QueryContext(ctx, `
SELECT `+sqliteCommitJoinCols+`
WHERE m.name = ?
ORDER BY c.create_time DESC, c.rowid DESC
LIMIT 1`, ref.Owner+"/"+ref.Module)
		}
		if err != nil {
			return nil, err
		}
		result, err := scanSQLiteCommitRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		commits = append(commits, result...)
	}
	return commits, nil
}

func (c *SQLiteCommitStorage) ListByModule(ctx context.Context, moduleID string, limit, offset int) ([]*registryv1.Commit, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	moduleID = sqlutil.ID(moduleID)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	// rowid tiebreaks, as in GetCommitByOwnerModule: create_time has
	// one-second resolution, and without a second key a page boundary can drop
	// or repeat a commit when several land in the same second.
	rows, err := c.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteCommitJoinCols+` WHERE c.module_id = ? ORDER BY c.create_time DESC, c.rowid DESC LIMIT ? OFFSET ?`, moduleID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteCommitRows(rows)
}

func (c *SQLiteCommitStorage) GetByHash(ctx context.Context, commitHash string) (*registryv1.Commit, error) {
	rows, err := c.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteCommitJoinCols+` WHERE c.commit_hash = ?`, commitHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commits, err := scanSQLiteCommitRows(rows)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, commit.ErrNotFound
	}
	return commits[0], nil
}

func (c *SQLiteCommitStorage) GetByHashPrefix(ctx context.Context, prefix string) (*registryv1.Commit, error) {
	// The prefix is escaped and an ESCAPE clause declared. A caller-supplied
	// "%" would otherwise match everything and return an arbitrary commit to
	// someone who supplied no real prefix at all.
	rows, err := c.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteCommitJoinCols+` WHERE c.commit_hash LIKE ? || '%' ESCAPE '\' LIMIT 1`,
		sqlutil.LikePrefix(prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commits, err := scanSQLiteCommitRows(rows)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, commit.ErrNotFound
	}
	return commits[0], nil
}

func (c *SQLiteCommitStorage) DeleteByIds(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = sqlutil.ID(id)
	}
	_, err := c.q(ctx).ExecContext(ctx,
		`DELETE FROM commits WHERE id IN (`+placeholders+`)`, args...)
	return err
}

var _ commit.Storage = (*SQLiteCommitStorage)(nil)
