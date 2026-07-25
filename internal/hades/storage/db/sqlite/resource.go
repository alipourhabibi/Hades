package sqlite

import (
	"context"
	"database/sql"
	"errors"

	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// SQLiteResourceStorage implements resource.Storage using database/sql with SQLite.
type SQLiteResourceStorage struct {
	db *sql.DB
}

func NewResource(db *sql.DB) *SQLiteResourceStorage {
	return &SQLiteResourceStorage{db: db}
}

func (r *SQLiteResourceStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return r.db
}

func (r *SQLiteResourceStorage) ResolveType(ctx context.Context, id string) (resource.ResourceType, error) {
	var rt string
	// REPLACE normalises dashes so dashless UUIDs (from buf CLI) match stored dashed UUIDs.
	err := r.q(ctx).QueryRowContext(ctx,
		"SELECT resource_type FROM resources WHERE REPLACE(id,'-','') = REPLACE(?1,'-','')", id,
	).Scan(&rt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", pkgerr.New("resource not found: "+id, pkgerr.NotFound)
	}
	if err != nil {
		return "", err
	}
	return resource.ResourceType(rt), nil
}

func (r *SQLiteResourceStorage) Register(ctx context.Context, id string, rt resource.ResourceType) error {
	_, err := r.q(ctx).ExecContext(ctx,
		"INSERT OR IGNORE INTO resources (id, resource_type) VALUES (?1, ?2)",
		id, string(rt),
	)
	return err
}
