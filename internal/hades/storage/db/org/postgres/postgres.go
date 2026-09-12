// Package postgres provides the PostgreSQL implementation of org.Storage.
package postgres

import (
	"context"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// OrgStorage implements org.Storage against PostgreSQL.
type OrgStorage struct {
	pool *pgxpool.Pool
}

// New creates an OrgStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *OrgStorage {
	return &OrgStorage{pool: pool}
}

var _ org.Storage = (*OrgStorage)(nil)

func (s *OrgStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

// The password column is deliberately absent from every read below.
// identityv1.User is what read handlers return to the caller, so a hash loaded
// into User.Password would travel out over the wire.
const userSelectColumns = `
  id, create_time, update_time, username, email, type, state, description, url`

const userSelectColumnsQualified = `
  u.id, u.create_time, u.update_time, u.username, u.email, u.type, u.state, u.description, u.url`

const returningUserColumns = `id, create_time, update_time, username, email, type, state, description, url`

func scanUser(row pgx.Row) (*identityv1.User, error) {
	usr := &identityv1.User{}
	var createTime, updateTime time.Time
	err := row.Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	if err != nil {
		return nil, err
	}
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	return usr, nil
}

func (s *OrgStorage) GetByName(ctx context.Context, name string) (*identityv1.User, error) {
	query := `SELECT` + userSelectColumns + `
FROM users
WHERE username = $1 AND type = 1`
	row := s.q(ctx).QueryRow(ctx, query, name)
	return scanUser(row)
}

func (s *OrgStorage) List(ctx context.Context, query string) ([]*identityv1.User, error) {
	rows, err := s.q(ctx).Query(ctx, `
SELECT`+userSelectColumns+`
FROM users
WHERE type = 1
  AND ($1 = '' OR username ILIKE '%' || $1 || '%')
ORDER BY username
LIMIT 50`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*identityv1.User
	for rows.Next() {
		usr := &identityv1.User{}
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&usr.Id, &createTime, &updateTime,
			&usr.Username, &usr.Email,
			&usr.Type, &usr.State, &usr.Description, &usr.Url,
		); err != nil {
			return nil, err
		}
		usr.CreateTime = timestamppb.New(createTime)
		usr.UpdateTime = timestamppb.New(updateTime)
		orgs = append(orgs, usr)
	}
	return orgs, rows.Err()
}

func (s *OrgStorage) Create(ctx context.Context, name, description, url, creatorID string) (*identityv1.User, error) {
	row := s.q(ctx).QueryRow(ctx, `
INSERT INTO users (username, email, password, type, state, description, url)
VALUES ($1, '', '', 1, 1, $2, $3)
RETURNING `+returningUserColumns,
		name, description, url,
	)
	o, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	_, err = s.q(ctx).Exec(ctx, `
INSERT INTO org_memberships (org_id, member_id, role)
VALUES ($1, $2, 'admin')
ON CONFLICT (org_id, member_id) DO UPDATE SET role = 'admin'`,
		o.Id, creatorID,
	)
	return o, err
}

func (s *OrgStorage) Update(ctx context.Context, orgID, description, url string) (*identityv1.User, error) {
	row := s.q(ctx).QueryRow(ctx, `
UPDATE users SET description=$1, url=$2, update_time=NOW()
WHERE id=$3
RETURNING `+returningUserColumns,
		description, url, orgID,
	)
	return scanUser(row)
}

func (s *OrgStorage) AddMember(ctx context.Context, orgID, memberID, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := s.q(ctx).Exec(ctx, `
INSERT INTO org_memberships (org_id, member_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (org_id, member_id) DO UPDATE SET role = $3`,
		orgID, memberID, role,
	)
	return err
}

func (s *OrgStorage) RemoveMember(ctx context.Context, orgID, memberID string) error {
	_, err := s.q(ctx).Exec(ctx,
		`DELETE FROM org_memberships WHERE org_id=$1 AND member_id=$2`,
		orgID, memberID,
	)
	return err
}

func (s *OrgStorage) GetUserOrgs(ctx context.Context, memberID string) ([]*identityv1.User, error) {
	rows, err := s.q(ctx).Query(ctx, `
SELECT`+userSelectColumnsQualified+`
FROM users u
JOIN org_memberships om ON u.id = om.org_id
WHERE om.member_id = $1
ORDER BY u.username`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*identityv1.User
	for rows.Next() {
		usr := &identityv1.User{}
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&usr.Id, &createTime, &updateTime,
			&usr.Username, &usr.Email,
			&usr.Type, &usr.State, &usr.Description, &usr.Url,
		); err != nil {
			return nil, err
		}
		usr.CreateTime = timestamppb.New(createTime)
		usr.UpdateTime = timestamppb.New(updateTime)
		orgs = append(orgs, usr)
	}
	return orgs, rows.Err()
}

func (s *OrgStorage) CountMembers(ctx context.Context, orgID string) (int32, error) {
	var count int32
	err := s.q(ctx).QueryRow(ctx,
		`SELECT COUNT(*) FROM org_memberships WHERE org_id = $1`, orgID,
	).Scan(&count)
	return count, err
}

func (s *OrgStorage) GetMemberRole(ctx context.Context, orgID, memberID string) (string, error) {
	var role string
	err := s.q(ctx).QueryRow(ctx,
		`SELECT role FROM org_memberships WHERE org_id=$1 AND member_id=$2`,
		orgID, memberID,
	).Scan(&role)
	if err != nil {
		// See the SQLite implementation: absence and failure must be
		// distinguishable, and both are the caller's to interpret.
		return "", err
	}
	return role, nil
}

func (s *OrgStorage) ListMembers(ctx context.Context, orgID string) ([]*org.OrgMember, error) {
	query := `
SELECT` + userSelectColumnsQualified + `, om.role
FROM users u
JOIN org_memberships om ON u.id = om.member_id
WHERE om.org_id = $1
ORDER BY u.username`

	rows, err := s.q(ctx).Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*org.OrgMember
	for rows.Next() {
		usr := &identityv1.User{}
		var createTime, updateTime time.Time
		var role string
		if err := rows.Scan(
			&usr.Id, &createTime, &updateTime,
			&usr.Username, &usr.Email,
			&usr.Type, &usr.State, &usr.Description, &usr.Url,
			&role,
		); err != nil {
			return nil, err
		}
		usr.CreateTime = timestamppb.New(createTime)
		usr.UpdateTime = timestamppb.New(updateTime)
		members = append(members, &org.OrgMember{User: usr, Role: role})
	}
	return members, rows.Err()
}
