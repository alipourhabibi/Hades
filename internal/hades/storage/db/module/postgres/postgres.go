package postgres

import (
	"context"
	"strings"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ModuleStorage executes module queries against PostgreSQL.
type ModuleStorage struct {
	pool *pgxpool.Pool
	res  resource.Storage
}

func New(pool *pgxpool.Pool, res resource.Storage) *ModuleStorage {
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
	lintPreset registryv1.LintPreset,
	breakingEnabled bool,
) (*registryv1.Module, error) {
	query := `
INSERT INTO modules (
  name, owner_id, visibility, state, description, url, default_label_name, default_branch, lint_preset, breaking_enabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, create_time, update_time, name, owner_id, visibility, state, description, url, default_label_name, default_branch, lint_preset, breaking_enabled`

	row := m.q(ctx).QueryRow(ctx, query, name, ownerId, visibility, state, description, url, defaultLabelName, defaultBranch, lintPreset, breakingEnabled)
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
		&mod.LintPreset, &mod.BreakingEnabled,
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
  modules.default_label_name, modules.default_branch,
  modules.lint_preset, modules.breaking_enabled
FROM modules`

func (m *ModuleStorage) Update(ctx context.Context, req *registryv1.UpdateModuleRequest) (*registryv1.Module, error) {
	// Convert optional enum pointers to *int32 so pgx sends NULL for unset fields.
	var vis, lint *int32
	if req.Visibility != nil {
		v := int32(*req.Visibility)
		vis = &v
	}
	if req.LintPreset != nil {
		v := int32(*req.LintPreset)
		lint = &v
	}
	query := `
UPDATE modules
SET
  description     = COALESCE($3, description),
  visibility      = COALESCE($4, visibility),
  lint_preset     = COALESCE($5, lint_preset),
  breaking_enabled = COALESCE($6, breaking_enabled),
  update_time     = now()
FROM users
WHERE users.id = modules.owner_id AND users.username = $1 AND modules.name = $2
RETURNING modules.id, modules.create_time, modules.update_time, modules.name, modules.owner_id,
          modules.visibility, modules.state, modules.description, modules.url,
          modules.default_label_name, modules.default_branch, modules.lint_preset, modules.breaking_enabled`

	row := m.q(ctx).QueryRow(ctx, query, req.Owner, req.Owner+"/"+req.Name, req.Description, vis, lint, req.BreakingEnabled)
	return scanModuleRow(row)
}

func (m *ModuleStorage) ListModules(ctx context.Context, ownerUsername string, limit, offset int) ([]*registryv1.Module, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var query string
	var args []interface{}
	if ownerUsername == "" {
		query = moduleSelectColumns + " ORDER BY modules.create_time DESC LIMIT $1 OFFSET $2"
		args = []interface{}{limit, offset}
	} else {
		query = moduleSelectColumns + `
JOIN users ON users.id = modules.owner_id
WHERE users.username = $1
ORDER BY modules.create_time DESC LIMIT $2 OFFSET $3`
		args = []interface{}{ownerUsername, limit, offset}
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
		return nil, connErr.NotFound("module not found")
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
			return nil, connErr.FromDB(err)
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

// visibilityPredicate narrows a module query to rows the caller may plausibly
// read. See module.Storage.ListVisibleModules: this is a pre-filter, and the
// OPA policy remains the authority.
//
// A module is included when it is public, when the caller owns it, or when the
// caller holds a role binding whose domain is either the module's full name or
// the module's owning namespace ("owner/*"). Those are exactly the domain forms
// AddBasicRoles, AddOrgOwner and AddOrgMemberBinding write.
const visibilityPredicate = `(
    modules.visibility = 1
    OR ($SUBJECT_ID <> '' AND modules.owner_id::text = $SUBJECT_ID)
    OR ($SUBJECT <> '' AND EXISTS (
        SELECT 1 FROM opa_role_bindings b
        WHERE b.subject = $SUBJECT
          AND (b.domain = modules.name OR b.domain = split_part(modules.name, '/', 1) || '/*')
    ))
)`

// ListVisibleModules implements module.Storage.
func (m *ModuleStorage) ListVisibleModules(ctx context.Context, ownerUsername, subject, subjectID string, limit, offset int) ([]*registryv1.Module, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	predicate := strings.NewReplacer("$SUBJECT_ID", "$1", "$SUBJECT", "$2").Replace(visibilityPredicate)

	var query string
	args := []interface{}{subjectID, subject}
	if ownerUsername == "" {
		query = moduleSelectColumns + " WHERE " + predicate + " ORDER BY modules.create_time DESC LIMIT $3 OFFSET $4"
		args = append(args, limit, offset)
	} else {
		query = moduleSelectColumns + `
JOIN users ON users.id = modules.owner_id
WHERE users.username = $3 AND ` + predicate + `
ORDER BY modules.create_time DESC LIMIT $4 OFFSET $5`
		args = append(args, ownerUsername, limit, offset)
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
