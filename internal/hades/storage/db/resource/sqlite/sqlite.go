package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
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
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	id = sqlutil.ID(id)
	var rt string
	// The parameter is normalised in Go, not the column in SQL. REPLACE on the
	// column made this primary-key lookup a full table scan of a table that
	// grows with every module and every push.
	err := r.q(ctx).QueryRowContext(ctx,
		"SELECT resource_type FROM resources WHERE id = ?1", id,
	).Scan(&rt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", resource.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return resource.ResourceType(rt), nil
}

func (r *SQLiteResourceStorage) Register(ctx context.Context, id string, rt resource.ResourceType) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	id = sqlutil.ID(id)
	_, err := r.q(ctx).ExecContext(ctx,
		"INSERT OR IGNORE INTO resources (id, resource_type) VALUES (?1, ?2)",
		sqlutil.ID(id), string(rt),
	)
	return err
}
