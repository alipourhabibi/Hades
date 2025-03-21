package module

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ModuleStorage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *ModuleStorage {
	return &ModuleStorage{
		pool: pool,
	}
}

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

	row := m.pool.QueryRow(ctx, query,
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
	rows, err := m.pool.Query(ctx, query, args...)
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
