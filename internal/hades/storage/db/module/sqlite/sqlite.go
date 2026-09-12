package sqlite

import (
	"context"
	"database/sql"
	"errors"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteModuleStorage implements module.Storage using database/sql with SQLite.
type SQLiteModuleStorage struct {
	db  *sql.DB
	res resource.Storage
}

func NewModule(db *sql.DB, res resource.Storage) *SQLiteModuleStorage {
	return &SQLiteModuleStorage{db: db, res: res}
}

func (m *SQLiteModuleStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return m.db
}

const sqliteModuleCols = `modules.id, modules.create_time, modules.update_time, modules.name, modules.owner_id, modules.visibility, modules.state, modules.description, modules.url, modules.default_label_name, modules.default_branch, modules.lint_preset, modules.breaking_enabled`

func scanSQLiteModule(row *sql.Row) (*registryv1.Module, error) {
	mod := &registryv1.Module{}
	var createTime, updateTime sqltypes.Time
	var breakingEnabled int
	err := row.Scan(
		&mod.Id, &createTime, &updateTime, &mod.Name, &mod.OwnerId,
		&mod.Visibility, &mod.State, &mod.Description, &mod.Url,
		&mod.DefaultLabelName, &mod.DefaultBranch,
		&mod.LintPreset, &breakingEnabled,
	)
	if err != nil {
		return nil, err
	}
	mod.Id = sqlutil.Canonical(mod.Id)
	mod.OwnerId = sqlutil.Canonical(mod.OwnerId)
	mod.CreateTime = timestamppb.New(createTime.V)
	mod.UpdateTime = timestamppb.New(updateTime.V)
	mod.BreakingEnabled = breakingEnabled != 0
	return mod, nil
}

func scanSQLiteModuleRow(rows *sql.Rows) (*registryv1.Module, error) {
	mod := &registryv1.Module{}
	var createTime, updateTime sqltypes.Time
	var breakingEnabled int
	err := rows.Scan(
		&mod.Id, &createTime, &updateTime, &mod.Name, &mod.OwnerId,
		&mod.Visibility, &mod.State, &mod.Description, &mod.Url,
		&mod.DefaultLabelName, &mod.DefaultBranch,
		&mod.LintPreset, &breakingEnabled,
	)
	if err != nil {
		return nil, err
	}
	mod.Id = sqlutil.Canonical(mod.Id)
	mod.OwnerId = sqlutil.Canonical(mod.OwnerId)
	mod.CreateTime = timestamppb.New(createTime.V)
	mod.UpdateTime = timestamppb.New(updateTime.V)
	mod.BreakingEnabled = breakingEnabled != 0
	return mod, nil
}

func (m *SQLiteModuleStorage) Create(ctx context.Context, name, ownerId string, visibility registryv1.ModuleVisibility, state registryv1.ModuleState, description, url, defaultLabelName, defaultBranch string, lintPreset registryv1.LintPreset, breakingEnabled bool) (*registryv1.Module, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	ownerId = sqlutil.ID(ownerId)
	breakingInt := 0
	if breakingEnabled {
		breakingInt = 1
	}
	_, err := m.q(ctx).ExecContext(ctx, `
INSERT INTO modules (name, owner_id, visibility, state, description, url, default_label_name, default_branch, lint_preset, breaking_enabled)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, ownerId, visibility, state, description, url, defaultLabelName, defaultBranch, lintPreset, breakingInt)
	if err != nil {
		return nil, err
	}
	mod, err := scanSQLiteModule(m.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteModuleCols+` FROM modules WHERE name = ?`, name))
	if err != nil {
		return nil, err
	}
	if err := m.res.Register(ctx, mod.Id, resource.ResourceTypeModule); err != nil {
		return nil, err
	}
	return mod, nil
}

func (m *SQLiteModuleStorage) Update(ctx context.Context, req *registryv1.UpdateModuleRequest) (*registryv1.Module, error) {
	// Pass nil for unset optional fields so COALESCE preserves the existing value.
	var vis, lint interface{}
	if req.Visibility != nil {
		vis = int64(*req.Visibility)
	}
	if req.LintPreset != nil {
		lint = int64(*req.LintPreset)
	}
	var breaking interface{}
	if req.BreakingEnabled != nil {
		if *req.BreakingEnabled {
			breaking = 1
		} else {
			breaking = 0
		}
	}
	_, err := m.q(ctx).ExecContext(ctx, `
UPDATE modules
SET
  description      = COALESCE(?, description),
  visibility       = COALESCE(?, visibility),
  lint_preset      = COALESCE(?, lint_preset),
  breaking_enabled = COALESCE(?, breaking_enabled),
  update_time      = datetime('now')
WHERE id = (
  SELECT modules.id FROM modules
  JOIN users ON users.id = modules.owner_id
  WHERE users.username = ? AND modules.name = ?
)`, req.Description, vis, lint, breaking, req.Owner, req.Owner+"/"+req.Name)
	if err != nil {
		return nil, err
	}
	return scanSQLiteModule(m.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteModuleCols+`
FROM modules
JOIN users ON users.id = modules.owner_id
WHERE users.username = ? AND modules.name = ?`, req.Owner, req.Owner+"/"+req.Name))
}

func (m *SQLiteModuleStorage) ListModules(ctx context.Context, ownerUsername string, limit, offset int) ([]*registryv1.Module, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var (
		rows *sql.Rows
		err  error
	)
	if ownerUsername == "" {
		rows, err = m.q(ctx).QueryContext(ctx,
			`SELECT `+sqliteModuleCols+` FROM modules ORDER BY create_time DESC LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = m.q(ctx).QueryContext(ctx, `
SELECT `+sqliteModuleCols+`
FROM modules
JOIN users ON users.id = modules.owner_id
WHERE users.username = ?
ORDER BY modules.create_time DESC LIMIT ? OFFSET ?`, ownerUsername, limit, offset)
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
		return nil, connerr.NotFound("module not found")
	}
	return scanSQLiteModuleRow(rows)
}

func (m *SQLiteModuleStorage) GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error) {
	var modules []*registryv1.Module
	for _, ref := range refs {
		var row *sql.Row
		if ref.Id != "" {
			row = m.q(ctx).QueryRowContext(ctx,
				`SELECT `+sqliteModuleCols+` FROM modules WHERE id = ?`, sqlutil.ID(ref.Id))
		} else {
			row = m.q(ctx).QueryRowContext(ctx,
				`SELECT `+sqliteModuleCols+` FROM modules WHERE name = ?`, ref.Owner+"/"+ref.Module)
		}
		mod, err := scanSQLiteModule(row)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, connerr.NotFound("module not found")
			}
			return nil, err
		}
		modules = append(modules, mod)
	}
	return modules, nil
}

func (m *SQLiteModuleStorage) CountByOwner(ctx context.Context, ownerID string) (int32, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	ownerID = sqlutil.ID(ownerID)
	var count int32
	err := m.q(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM modules WHERE owner_id = ?`, ownerID).Scan(&count)
	return count, err
}

var _ module.Storage = (*SQLiteModuleStorage)(nil)

// visibilityPredicate narrows a module query to rows the caller may plausibly
// read. See module.Storage.ListVisibleModules: this is a pre-filter, and the
// OPA policy remains the authority.
//
// A module is included when it is public, when the caller owns it, or when the
// caller holds a role binding whose domain is either the module's full name or
// the module's owning namespace ("owner/*"). Those are exactly the domain forms
// AddBasicRoles, AddOrgOwner and AddOrgMemberBinding write.
const sqliteVisibilityPredicate = `(
    modules.visibility = 1
    OR (? <> '' AND modules.owner_id = ?)
    OR (? <> '' AND EXISTS (
        SELECT 1 FROM opa_role_bindings b
        WHERE b.subject = ?
          AND (b.domain = modules.name
               OR (instr(modules.name, '/') > 0
                   AND b.domain = substr(modules.name, 1, instr(modules.name, '/')) || '*'))
    ))
)`

// ListVisibleModules implements module.Storage.
func (m *SQLiteModuleStorage) ListVisibleModules(ctx context.Context, ownerUsername, subject, subjectID string, limit, offset int) ([]*registryv1.Module, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	subjectID = sqlutil.ID(subjectID)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// The predicate names each parameter twice, so every value is bound twice.
	args := []interface{}{subjectID, subjectID, subject, subject}

	var query string
	if ownerUsername == "" {
		query = `SELECT ` + sqliteModuleCols + ` FROM modules WHERE ` + sqliteVisibilityPredicate +
			` ORDER BY modules.create_time DESC LIMIT ? OFFSET ?`
	} else {
		query = `SELECT ` + sqliteModuleCols + `
FROM modules
JOIN users ON users.id = modules.owner_id
WHERE users.username = ? AND ` + sqliteVisibilityPredicate + `
ORDER BY modules.create_time DESC LIMIT ? OFFSET ?`
		args = append([]interface{}{ownerUsername}, args...)
	}
	args = append(args, limit, offset)

	rows, err := m.q(ctx).QueryContext(ctx, query, args...)
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
