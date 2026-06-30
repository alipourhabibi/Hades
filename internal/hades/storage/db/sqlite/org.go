package sqlite

import (
	"context"
	"database/sql"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteOrgStorage implements org.Storage using database/sql with SQLite.
type SQLiteOrgStorage struct {
	db *sql.DB
}

func NewOrg(db *sql.DB) *SQLiteOrgStorage {
	return &SQLiteOrgStorage{db: db}
}

func (s *SQLiteOrgStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

const sqliteUserCols = `id, create_time, update_time, username, email, password, type, state, description, url`

func scanSQLiteOrgUser(row *sql.Row) (*registryv1.User, error) {
	u := &registryv1.User{}
	var createTime, updateTime time.Time
	var password sql.NullString
	err := row.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &password, &u.Type, &u.State, &u.Description, &u.Url)
	if err != nil {
		return nil, err
	}
	u.Password = password.String
	u.CreateTime = timestamppb.New(createTime)
	u.UpdateTime = timestamppb.New(updateTime)
	return u, nil
}

func (s *SQLiteOrgStorage) GetByName(ctx context.Context, name string) (*registryv1.User, error) {
	return scanSQLiteOrgUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE username = ? AND type = 1`, name))
}

func (s *SQLiteOrgStorage) List(ctx context.Context, query string) ([]*registryv1.User, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE type = 1 AND (? = '' OR username LIKE '%' || ? || '%') ORDER BY username LIMIT 50`, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteOrgRows(rows)
}

func scanSQLiteOrgRows(rows *sql.Rows) ([]*registryv1.User, error) {
	var orgs []*registryv1.User
	for rows.Next() {
		u := &registryv1.User{}
		var createTime, updateTime time.Time
		var password sql.NullString
		if err := rows.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &password, &u.Type, &u.State, &u.Description, &u.Url); err != nil {
			return nil, err
		}
		u.Password = password.String
		u.CreateTime = timestamppb.New(createTime)
		u.UpdateTime = timestamppb.New(updateTime)
		orgs = append(orgs, u)
	}
	return orgs, rows.Err()
}

func (s *SQLiteOrgStorage) Create(ctx context.Context, name, description, url, creatorID string) (*registryv1.User, error) {
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO users (username, email, password, type, state, description, url) VALUES (?, '', '', 1, 1, ?, ?)`,
		name, description, url)
	if err != nil {
		return nil, err
	}
	orgUser, err := scanSQLiteOrgUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE username = ?`, name))
	if err != nil {
		return nil, err
	}
	_, err = s.q(ctx).ExecContext(ctx,
		`INSERT OR REPLACE INTO org_memberships (org_id, member_id, role) VALUES (?, ?, 'admin')`, orgUser.Id, creatorID)
	return orgUser, err
}

func (s *SQLiteOrgStorage) Update(ctx context.Context, orgID, description, url string) (*registryv1.User, error) {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET description=?, url=?, update_time=datetime('now') WHERE id=?`, description, url, orgID)
	if err != nil {
		return nil, err
	}
	return scanSQLiteOrgUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE id = ?`, orgID))
}

func (s *SQLiteOrgStorage) AddMember(ctx context.Context, orgID, memberID, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT OR REPLACE INTO org_memberships (org_id, member_id, role) VALUES (?, ?, ?)`, orgID, memberID, role)
	return err
}

func (s *SQLiteOrgStorage) RemoveMember(ctx context.Context, orgID, memberID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`DELETE FROM org_memberships WHERE org_id=? AND member_id=?`, orgID, memberID)
	return err
}

func (s *SQLiteOrgStorage) GetUserOrgs(ctx context.Context, memberID string) ([]*registryv1.User, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT u.`+sqliteUserCols+`
FROM users u
JOIN org_memberships om ON u.id = om.org_id
WHERE om.member_id = ?
ORDER BY u.username`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteOrgRows(rows)
}

func (s *SQLiteOrgStorage) CountMembers(ctx context.Context, orgID string) (int32, error) {
	var count int32
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_memberships WHERE org_id = ?`, orgID).Scan(&count)
	return count, err
}

func (s *SQLiteOrgStorage) GetMemberRole(ctx context.Context, orgID, memberID string) (string, error) {
	var role string
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT role FROM org_memberships WHERE org_id=? AND member_id=?`, orgID, memberID).Scan(&role)
	if err != nil {
		return "", nil
	}
	return role, nil
}

func (s *SQLiteOrgStorage) ListMembers(ctx context.Context, orgID string) ([]*org.OrgMember, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT u.`+sqliteUserCols+`, om.role
FROM users u
JOIN org_memberships om ON u.id = om.member_id
WHERE om.org_id = ?
ORDER BY u.username`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []*org.OrgMember
	for rows.Next() {
		u := &registryv1.User{}
		var createTime, updateTime time.Time
		var password sql.NullString
		var role string
		if err := rows.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &password, &u.Type, &u.State, &u.Description, &u.Url, &role); err != nil {
			return nil, err
		}
		u.Password = password.String
		u.CreateTime = timestamppb.New(createTime)
		u.UpdateTime = timestamppb.New(updateTime)
		members = append(members, &org.OrgMember{User: u, Role: role})
	}
	return members, rows.Err()
}

var _ org.Storage = (*SQLiteOrgStorage)(nil)
