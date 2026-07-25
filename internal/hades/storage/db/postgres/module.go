package postgres

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ModuleStorage executes module queries against PostgreSQL.
type ModuleStorage struct {
	pool *pgxpool.Pool
	res  resource.Storage
}

func NewModule(pool *pgxpool.Pool, res resource.Storage) *ModuleStorage {
	return &ModuleStorage{pool: pool, res: res}
}

func (m *ModuleStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return m.pool
}

func (m *ModuleStorage) Create(
	ctx context.Context,
	name, ownerId string,
	visibility registryv1.ModuleVisibility,
	state registryv1.ModuleState,
	description, url, defaultLabelName, defaultBranch string,
) (*registryv1.Module, error) {
	query := `
INSERT INTO modules (
  name, owner_id, visibility, state, description, url, default_label_name, default_branch
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, create_time, update_time, name, owner_id, visibility, state, description, url, default_label_name, default_branch`

	row := m.q(ctx).QueryRow(ctx, query, name, ownerId, visibility, state, description, url, defaultLabelName, defaultBranch)
	mod, err := scanModuleRow(row)
	if err != nil {
		return nil, err
	}
	if err := m.res.Register(ctx, mod.Id, resource.ResourceTypeModule); err != nil {
		return nil, err
	}
	return mod, nil
}

func scanModuleRow(row interface {
	Scan(dest ...any) error
}) (*registryv1.Module, error) {
	mod := &registryv1.Module{}
	var createTime, updateTime time.Time
	err := row.Scan(
		&mod.Id, &createTime, &updateTime,
		&mod.Name, &mod.OwnerId,
		&mod.Visibility, &mod.State,
		&mod.Description, &mod.Url,
		&mod.DefaultLabelName, &mod.DefaultBranch,
	)
	if err != nil {
		return nil, err
	}
	mod.CreateTime = timestamppb.New(createTime)
	mod.UpdateTime = timestamppb.New(updateTime)
	return mod, nil
}

const moduleSelectColumns = `
SELECT
  modules.id, modules.create_time, modules.update_time,
  modules.name, modules.owner_id,
  modules.visibility, modules.state,
  modules.description, modules.url,
  modules.default_label_name, modules.default_branch
FROM modules`

func (m *ModuleStorage) ListModules(ctx context.Context, ownerUsername string) ([]*registryv1.Module, error) {
	var query string
	var args []interface{}
	if ownerUsername == "" {
		query = moduleSelectColumns + " ORDER BY modules.create_time DESC"
	} else {
		query = moduleSelectColumns + `
JOIN users ON users.id = modules.owner_id
WHERE users.username = $1
ORDER BY modules.create_time DESC`
		args = append(args, ownerUsername)
	}

	rows, err := m.q(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []*registryv1.Module
	for rows.Next() {
		mod, err := scanModuleRow(rows)
		if err != nil {
			return nil, err
		}
		modules = append(modules, mod)
	}
	return modules, rows.Err()
}

func (m *ModuleStorage) GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registryv1.Module, error) {
	query := moduleSelectColumns + `
JOIN users ON users.id = modules.owner_id
WHERE users.username = $1 AND modules.name = $2`

	rows, err := m.q(ctx).Query(ctx, query, owner, owner+"/"+name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("module not found")
	}
	return scanModuleRow(rows)
}

func (m *ModuleStorage) GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	var modules []*registryv1.Module
	for _, ref := range refs {
		var row pgx.Row
		if ref.Id != "" {
			row = m.q(ctx).QueryRow(ctx,
				moduleSelectColumns+` WHERE modules.id = $1`, ref.Id)
		} else {
			row = m.q(ctx).QueryRow(ctx,
				moduleSelectColumns+` WHERE modules.name = $1`, ref.Owner+"/"+ref.Module)
		}
		mod, err := scanModuleRow(row)
		if err != nil {
			return nil, err
		}
		modules = append(modules, mod)
	}
	return modules, nil
}

func (m *ModuleStorage) CountByOwner(ctx context.Context, ownerID string) (int32, error) {
	var count int32
	err := m.q(ctx).QueryRow(ctx,
		`SELECT COUNT(*) FROM modules WHERE owner_id = $1`, ownerID,
	).Scan(&count)
	return count, err
}

var _ module.Storage = (*ModuleStorage)(nil)
