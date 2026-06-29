package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteModuleStorage implements module.Storage using database/sql with SQLite.
type SQLiteModuleStorage struct {
	db *sql.DB
}

func NewModule(db *sql.DB) *SQLiteModuleStorage {
	return &SQLiteModuleStorage{db: db}
}

func (m *SQLiteModuleStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return m.db
}

const sqliteModuleCols = `modules.id, modules.create_time, modules.update_time, modules.name, modules.owner_id, modules.visibility, modules.state, modules.description, modules.url, modules.default_label_name, modules.default_branch`

func scanSQLiteModule(row *sql.Row) (*registryv1.Module, error) {
	mod := &registryv1.Module{}
	var createTime, updateTime sqltypes.Time
	err := row.Scan(
		&mod.Id, &createTime, &updateTime, &mod.Name, &mod.OwnerId,
		&mod.Visibility, &mod.State, &mod.Description, &mod.Url,
		&mod.DefaultLabelName, &mod.DefaultBranch,
	)
	if err != nil {
		return nil, err
	}
	mod.CreateTime = timestamppb.New(createTime.V)
	mod.UpdateTime = timestamppb.New(updateTime.V)
	return mod, nil
}

func scanSQLiteModuleRow(rows *sql.Rows) (*registryv1.Module, error) {
	mod := &registryv1.Module{}
	var createTime, updateTime sqltypes.Time
	err := rows.Scan(
		&mod.Id, &createTime, &updateTime, &mod.Name, &mod.OwnerId,
		&mod.Visibility, &mod.State, &mod.Description, &mod.Url,
		&mod.DefaultLabelName, &mod.DefaultBranch,
	)
	if err != nil {
		return nil, err
	}
	mod.CreateTime = timestamppb.New(createTime.V)
	mod.UpdateTime = timestamppb.New(updateTime.V)
	return mod, nil
}

func (m *SQLiteModuleStorage) Create(ctx context.Context, name, ownerId string, visibility registryv1.ModuleVisibility, state registryv1.ModuleState, description, url, defaultLabelName, defaultBranch string) (*registryv1.Module, error) {
	_, err := m.q(ctx).ExecContext(ctx, `
INSERT INTO modules (name, owner_id, visibility, state, description, url, default_label_name, default_branch)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		name, ownerId, visibility, state, description, url, defaultLabelName, defaultBranch)
	if err != nil {
		return nil, err
	}
	return scanSQLiteModule(m.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteModuleCols+` FROM modules WHERE name = ?`, name))
}

func (m *SQLiteModuleStorage) ListModules(ctx context.Context, ownerUsername string) ([]*registryv1.Module, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if ownerUsername == "" {
		rows, err = m.q(ctx).QueryContext(ctx,
			`SELECT `+sqliteModuleCols+` FROM modules ORDER BY create_time DESC`)
	} else {
		rows, err = m.q(ctx).QueryContext(ctx, `
SELECT `+sqliteModuleCols+`
FROM modules
JOIN users ON users.id = modules.owner_id
WHERE users.username = ?
ORDER BY modules.create_time DESC`, ownerUsername)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var modules []*registryv1.Module
	for rows.Next() {
		mod, err := scanSQLiteModuleRow(rows)
		if err != nil {
			return nil, err
		}
		modules = append(modules, mod)
	}
	return modules, rows.Err()
}

func (m *SQLiteModuleStorage) GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registryv1.Module, error) {
	rows, err := m.q(ctx).QueryContext(ctx, `
SELECT `+sqliteModuleCols+`
FROM modules
JOIN users ON users.id = modules.owner_id
WHERE users.username = ? AND modules.name = ?`, owner, owner+"/"+name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("module not found")
	}
	return scanSQLiteModuleRow(rows)
}

func (m *SQLiteModuleStorage) GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	var modules []*registryv1.Module
	for _, ref := range refs {
		var row *sql.Row
		if ref.Id != "" {
			row = m.q(ctx).QueryRowContext(ctx,
				`SELECT `+sqliteModuleCols+` FROM modules WHERE id = ?`, ref.Id)
		} else {
			row = m.q(ctx).QueryRowContext(ctx,
				`SELECT `+sqliteModuleCols+` FROM modules WHERE name = ?`, ref.Owner+"/"+ref.Module)
		}
		mod, err := scanSQLiteModule(row)
		if err != nil {
			return nil, err
		}
		modules = append(modules, mod)
	}
	return modules, nil
}

func (m *SQLiteModuleStorage) CountByOwner(ctx context.Context, ownerID string) (int32, error) {
	var count int32
	err := m.q(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM modules WHERE owner_id = ?`, ownerID).Scan(&count)
	return count, err
}

var _ module.Storage = (*SQLiteModuleStorage)(nil)
