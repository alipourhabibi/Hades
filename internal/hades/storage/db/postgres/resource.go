package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// ResourceStorage implements resource.Storage against PostgreSQL.
type ResourceStorage struct {
	pool *pgxpool.Pool
}

func NewResource(pool *pgxpool.Pool) *ResourceStorage {
	return &ResourceStorage{pool: pool}
}

func (r *ResourceStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

func (r *ResourceStorage) ResolveType(ctx context.Context, id string) (resource.ResourceType, error) {
	var rt string
	err := r.q(ctx).QueryRow(ctx,
		"SELECT resource_type FROM resources WHERE id = $1", id,
	).Scan(&rt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pkgerr.New("resource not found: "+id, pkgerr.NotFound)
	}
	if err != nil {
		return "", err
	}
	return resource.ResourceType(rt), nil
}

func (r *ResourceStorage) Register(ctx context.Context, id string, rt resource.ResourceType) error {
	_, err := r.q(ctx).Exec(ctx,
		"INSERT INTO resources (id, resource_type) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		id, string(rt),
	)
	return err
}
