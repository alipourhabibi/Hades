package sqlite

import (
	"context"
	"database/sql"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
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

// sqliteUserCols deliberately omits the password column. identityv1.User is
// what every read handler returns to the caller, so a hash loaded into
// User.Password would travel out over the wire.
const sqliteUserCols = `id, create_time, update_time, username, email, type, state, description, url`

// Timestamps are scanned through sqltypes.Time, not time.Time. modernc's
// driver returns them as strings, which is what sqltypes exists to parse; a
// bare time.Time worked only for the spellings the driver happened to hand
// back as time values.
func scanSQLiteOrgUser(row *sql.Row) (*identityv1.User, error) {
	u := &identityv1.User{}
	var createTime, updateTime sqltypes.Time
	err := row.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &u.Type, &u.State, &u.Description, &u.Url)
	if err != nil {
		return nil, err
	}
	u.Id = sqlutil.Canonical(u.Id)
	u.CreateTime = timestamppb.New(createTime.V)
	u.UpdateTime = timestamppb.New(updateTime.V)
	return u, nil
}

func (s *SQLiteOrgStorage) GetByName(ctx context.Context, name string) (*identityv1.User, error) {
	return scanSQLiteOrgUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE username = ? AND type = 1`, name))
}

func (s *SQLiteOrgStorage) List(ctx context.Context, query string, limit, offset int) ([]*identityv1.User, error) {
	// One backslash in the ESCAPE literal, not two. The query is a raw string,
	// where there are no escape sequences, so "\\" is two characters and ESCAPE
	// takes exactly one.
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteUserCols+`
		 FROM users
		 WHERE type = 1 AND (? = '' OR username LIKE '%' || ? || '%' ESCAPE '\')
		 ORDER BY username LIMIT ? OFFSET ?`,
		query, sqlutil.LikePrefix(query), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteOrgRows(rows)
}

func scanSQLiteOrgRows(rows *sql.Rows) ([]*identityv1.User, error) {
	var orgs []*identityv1.User
	for rows.Next() {
		u := &identityv1.User{}
		var createTime, updateTime sqltypes.Time
		if err := rows.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &u.Type, &u.State, &u.Description, &u.Url); err != nil {
			return nil, err
		}
		u.Id = sqlutil.Canonical(u.Id)
		u.CreateTime = timestamppb.New(createTime.V)
		u.UpdateTime = timestamppb.New(updateTime.V)
		orgs = append(orgs, u)
	}
	return orgs, rows.Err()
}

func (s *SQLiteOrgStorage) Create(ctx context.Context, name, description, url, creatorID string) (*identityv1.User, error) {
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
		`INSERT INTO org_memberships (org_id, member_id, role) VALUES (?, ?, 'admin')
		 ON CONFLICT(org_id, member_id) DO UPDATE SET role = excluded.role`,
		sqlutil.ID(orgUser.Id), sqlutil.ID(creatorID))
	return orgUser, err
}

func (s *SQLiteOrgStorage) Update(ctx context.Context, orgID, description, url string) (*identityv1.User, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET description=?, url=?, update_time=datetime('now') WHERE id=?`, description, url, orgID)
	if err != nil {
		return nil, err
	}
	return scanSQLiteOrgUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserCols+` FROM users WHERE id = ?`, orgID))
}

func (s *SQLiteOrgStorage) AddMember(ctx context.Context, orgID, memberID, role string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	memberID = sqlutil.ID(memberID)
	if role == "" {
		role = "member"
	}
	// ON CONFLICT DO UPDATE, not INSERT OR REPLACE. REPLACE is delete then
	// insert, so it discarded the row's id and created_at and re-created it
	// with new ones, where PostgreSQL updated in place.
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO org_memberships (org_id, member_id, role) VALUES (?, ?, ?)
		 ON CONFLICT(org_id, member_id) DO UPDATE SET role = excluded.role`,
		orgID, memberID, role)
	return err
}

func (s *SQLiteOrgStorage) RemoveMember(ctx context.Context, orgID, memberID string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	memberID = sqlutil.ID(memberID)
	_, err := s.q(ctx).ExecContext(ctx,
		`DELETE FROM org_memberships WHERE org_id=? AND member_id=?`, orgID, memberID)
	return err
}

func (s *SQLiteOrgStorage) GetUserOrgs(ctx context.Context, memberID string) ([]*identityv1.User, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	memberID = sqlutil.ID(memberID)
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT u.`+sqliteUserCols+`
FROM users u
JOIN org_memberships om ON u.id = om.org_id
WHERE om.member_id = ?
ORDER BY u.username LIMIT 1000`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteOrgRows(rows)
}

func (s *SQLiteOrgStorage) CountMembers(ctx context.Context, orgID string) (int32, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	var count int32
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_memberships WHERE org_id = ?`, orgID).Scan(&count)
	return count, err
}

func (s *SQLiteOrgStorage) GetMemberRole(ctx context.Context, orgID, memberID string) (string, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	memberID = sqlutil.ID(memberID)
	var role string
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT role FROM org_memberships WHERE org_id=? AND member_id=?`, orgID, memberID).Scan(&role)
	if err != nil {
		// The error is returned, not swallowed. Reporting ("", nil) for a
		// missing row made "not a member" indistinguishable from "found, with
		// no role", so RemoveOrgMember's NOT_FOUND branch could never fire and
		// a database failure looked like a definite answer.
		return "", err
	}
	return role, nil
}

func (s *SQLiteOrgStorage) ListMembers(ctx context.Context, orgID string) ([]*org.OrgMember, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	orgID = sqlutil.ID(orgID)
	rows, err := s.q(ctx).QueryContext(ctx, `
SELECT u.`+sqliteUserCols+`, om.role
FROM users u
JOIN org_memberships om ON u.id = om.member_id
WHERE om.org_id = ?
ORDER BY u.username LIMIT 1000`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []*org.OrgMember
	for rows.Next() {
		u := &identityv1.User{}
		var createTime, updateTime sqltypes.Time
		var role string
		if err := rows.Scan(&u.Id, &createTime, &updateTime, &u.Username, &u.Email, &u.Type, &u.State, &u.Description, &u.Url, &role); err != nil {
			return nil, err
		}
		u.Id = sqlutil.Canonical(u.Id)
		u.CreateTime = timestamppb.New(createTime.V)
		u.UpdateTime = timestamppb.New(updateTime.V)
		members = append(members, &org.OrgMember{User: u, Role: role})
	}
	return members, rows.Err()
}

var _ org.Storage = (*SQLiteOrgStorage)(nil)
