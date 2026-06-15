// Package module provides PostgreSQL storage for module metadata and ownership lookups.
package module

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
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

// ModuleStorage executes module queries against PostgreSQL. It supports both
// pooled connections and explicit transactions via WithTx.
type ModuleStorage struct {
	db querier
}

// New returns a ModuleStorage backed by the given connection pool.
func New(pool *pgxpool.Pool) *ModuleStorage {
	return &ModuleStorage{
		db: pool,
	}
}

// WithTx returns a copy of ModuleStorage bound to the given transaction.
func (m *ModuleStorage) WithTx(tx pgx.Tx) *ModuleStorage {
	return &ModuleStorage{db: tx}
}

// Create inserts a new module row and returns it with server-assigned id and timestamps.
func (m *ModuleStorage) Create(
	ctx context.Context,
	name string,
	ownerId string,
	visibility registryv1.ModuleVisibility,
	state registryv1.ModuleState,
	description string,
	url string,
	defaultLabelName string,
	defaultBranch string,
) (*registryv1.Module, error) {

	query := `
INSERT INTO modules (
  name,
  owner_id,
  visibility,
  state,
  description,
  url,
  default_label_name,
  default_branch
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, create_time, update_time, name, owner_id, visibility, state, description, url, default_label_name, default_branch`

	row := m.db.QueryRow(ctx, query,
		name,
		ownerId,
		visibility,
		state,
		description,
		url,
		defaultLabelName,
		defaultBranch,
	)

	module := &registryv1.Module{}

	var createTime time.Time
	var updateTime time.Time
	err := row.Scan(
		&module.Id,
		&createTime,
		&updateTime,
		&module.Name,
		&module.OwnerId,
		&module.Visibility,
		&module.State,
		&module.Description,
		&module.Url,
		&module.DefaultLabelName,
		&module.DefaultBranch,
	)

	module.CreateTime = timestamppb.New(createTime)
	module.UpdateTime = timestamppb.New(updateTime)

	if err != nil {
		return nil, err
	}

	return module, nil
}

// scanModuleRow scans a single module row (from the standard select column
// list defined by moduleSelectColumns) into a *registryv1.Module.
func scanModuleRow(rows pgx.Rows) (*registryv1.Module, error) {
	module := &registryv1.Module{}
	var createTime, updateTime time.Time
	err := rows.Scan(
		&module.Id,
		&createTime,
		&updateTime,
		&module.Name,
		&module.OwnerId,
		&module.Visibility,
		&module.State,
		&module.Description,
		&module.Url,
		&module.DefaultLabelName,
		&module.DefaultBranch,
	)
	if err != nil {
		return nil, err
	}
	module.CreateTime = timestamppb.New(createTime)
	module.UpdateTime = timestamppb.New(updateTime)
	return module, nil
}

// moduleSelectColumns is the standard SELECT … FROM modules clause used by
// ListModules and GetModuleByOwnerAndName.  All columns are qualified with
// the "modules." table prefix so they remain unambiguous when the query also
// JOINs the users table (which has its own id, create_time, update_time …).
const moduleSelectColumns = `
SELECT
  modules.id,
  modules.create_time,
  modules.update_time,
  modules.name,
  modules.owner_id,
  modules.visibility,
  modules.state,
  modules.description,
  modules.url,
  modules.default_label_name,
  modules.default_branch
FROM modules`

// ListModules returns all modules visible in the database, optionally
// filtered to a single owner.  When ownerUsername is empty all modules are
// returned regardless of owner; callers are responsible for applying any
// visibility / access-control filtering before returning results to clients.
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

	rows, err := m.db.Query(ctx, query, args...)
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

// GetModuleByOwnerAndName returns a single module identified by the owner's
// username and the module's short name (the part after the "/").
// Returns a "module not found" error if no matching row exists.
func (m *ModuleStorage) GetModuleByOwnerAndName(ctx context.Context, owner, name string) (*registryv1.Module, error) {
	query := moduleSelectColumns + `
JOIN users ON users.id = modules.owner_id
WHERE users.username = $1 AND modules.name = $2`

	rows, err := m.db.Query(ctx, query, owner, owner+"/"+name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("module not found")
	}
	return scanModuleRow(rows)
}

// GetModulesByRefs resolves one or more ModuleRef values (by id or owner/name)
// into full Module rows. All refs are ANDed into a single query.
func (m *ModuleStorage) GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {

	query := `
SELECT
  id,
  create_time,
  update_time,
  name,
  owner_id,
  visibility,
  state,
  description,
  url,
  default_label_name,
  default_branch
FROM modules WHERE `

	var conditions []string
	var args []interface{}
	argIndex := 1

	// Loop through the refs and build the conditions
	for _, req := range refs {
		if req.Id != "" {
			conditions = append(conditions, fmt.Sprintf("id = $%d", argIndex))
			args = append(args, req.Id)
			argIndex++
		} else {
			conditions = append(conditions, fmt.Sprintf("modules.name = $%d", argIndex))
			args = append(args, req.Owner+"/"+req.Module)
			argIndex++
		}
	}
	for i, f := range conditions {
		query += f
		if i < len(refs)-1 {
			query += " AND "
		}
	}

	// Execute the query
	rows, err := m.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Prepare a slice to hold the result
	var modules []*registryv1.Module

	// Scan the rows into the modules slice
	for rows.Next() {
		var module registryv1.Module
		var createTime time.Time
		var updateTime time.Time
		err := rows.Scan(
			&module.Id,
			&createTime,
			&updateTime,
			&module.Name,
			&module.OwnerId,
			&module.Visibility,
			&module.State,
			&module.Description,
			&module.Url,
			&module.DefaultLabelName,
			&module.DefaultBranch,
		)
		module.CreateTime = timestamppb.New(createTime)
		module.UpdateTime = timestamppb.New(updateTime)
		if err != nil {
			return nil, err
		}
		modules = append(modules, &module)
	}

	// Check if any error occurred during iteration
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return modules, nil
}

// CountByOwner returns the total number of modules owned by ownerID.
func (m *ModuleStorage) CountByOwner(ctx context.Context, ownerID string) (int32, error) {
	var count int32
	err := m.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM modules WHERE owner_id = $1`,
		ownerID,
	).Scan(&count)
	return count, err
}
