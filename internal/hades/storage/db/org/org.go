// Package org provides storage operations for organization users and their
// membership records.  Organizations are regular users whose type field is
// set to USER_TYPE_ORGANIZATION; memberships are stored in the
// org_memberships join table (migration 019).
package org

import (
	"context"
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

// OrgMember holds a user and their role in an org.
type OrgMember struct {
	User *registryv1.User
	Role string
}

// OrgStorage handles organization-related queries against the users and
// org_memberships tables.
type OrgStorage struct {
	db querier
}

// New creates an OrgStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *OrgStorage {
	return &OrgStorage{db: pool}
}

// WithTx returns a shallow copy of OrgStorage that executes queries within
// the given transaction instead of the pool.
func (s *OrgStorage) WithTx(tx pgx.Tx) *OrgStorage {
	return &OrgStorage{db: tx}
}

const userSelectColumns = `
  id, create_time, update_time, username, email, password, type, state, description, url`

// userSelectColumnsQualified prefixes every column with the "u" table alias.
// Use this in JOIN queries where org_memberships also has an "id" column.
const userSelectColumnsQualified = `
  u.id, u.create_time, u.update_time, u.username, u.email, u.password, u.type, u.state, u.description, u.url`

// scanUser reads one user row from a pgx.Row.
func scanUser(row pgx.Row) (*registryv1.User, error) {
	user := &registryv1.User{}
	var createTime, updateTime time.Time
	err := row.Scan(
		&user.Id,
		&createTime,
		&updateTime,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Type,
		&user.State,
		&user.Description,
		&user.Url,
	)
	if err != nil {
		return nil, err
	}
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	return user, nil
}

// GetByName returns the organization user with the given username.
// Returns pgx.ErrNoRows if no organization with that name exists.
func (s *OrgStorage) GetByName(ctx context.Context, name string) (*registryv1.User, error) {
	query := `SELECT` + userSelectColumns + `
FROM users
WHERE username = $1 AND type = 1`
	row := s.db.QueryRow(ctx, query, name)
	return scanUser(row)
}

// List returns orgs whose username ILIKE '%query%'.
// Empty query returns the first 50 orgs ordered by username.
func (s *OrgStorage) List(ctx context.Context, query string) ([]*registryv1.User, error) {
	rows, err := s.db.Query(ctx, `
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

	var orgs []*registryv1.User
	for rows.Next() {
		user := &registryv1.User{}
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&user.Id, &createTime, &updateTime,
			&user.Username, &user.Email, &user.Password,
			&user.Type, &user.State, &user.Description, &user.Url,
		); err != nil {
			return nil, err
		}
		user.CreateTime = timestamppb.New(createTime)
		user.UpdateTime = timestamppb.New(updateTime)
		orgs = append(orgs, user)
	}
	return orgs, rows.Err()
}

const returningUserColumns = `id, create_time, update_time, username, email, password, type, state, description, url`

// Create inserts a new organization user row and adds creatorID as its first admin.
func (s *OrgStorage) Create(ctx context.Context, name, description, url, creatorID string) (*registryv1.User, error) {
	row := s.db.QueryRow(ctx, `
INSERT INTO users (username, email, password, type, state, description, url)
VALUES ($1, '', '', 1, 1, $2, $3)
RETURNING `+returningUserColumns,
		name, description, url,
	)
	org, err := scanUser(row)
	if err != nil {
		return nil, err
	}

	// Add the creator as admin.
	_, err = s.db.Exec(ctx, `
INSERT INTO org_memberships (org_id, member_id, role)
VALUES ($1, $2, 'admin')
ON CONFLICT (org_id, member_id) DO UPDATE SET role = 'admin'`,
		org.Id, creatorID,
	)
	return org, err
}

// Update sets description and url for the given org and returns the updated row.
func (s *OrgStorage) Update(ctx context.Context, orgID, description, url string) (*registryv1.User, error) {
	row := s.db.QueryRow(ctx, `
UPDATE users SET description=$1, url=$2, update_time=NOW()
WHERE id=$3
RETURNING `+returningUserColumns,
		description, url, orgID,
	)
	return scanUser(row)
}

// AddMember upserts a membership row with the given role.
func (s *OrgStorage) AddMember(ctx context.Context, orgID, memberID, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO org_memberships (org_id, member_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (org_id, member_id) DO UPDATE SET role = $3`,
		orgID, memberID, role,
	)
	return err
}

// RemoveMember deletes the membership row for the given org/member pair.
func (s *OrgStorage) RemoveMember(ctx context.Context, orgID, memberID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM org_memberships WHERE org_id=$1 AND member_id=$2`,
		orgID, memberID,
	)
	return err
}

// GetUserOrgs returns all organizations in which memberID has a membership row.
func (s *OrgStorage) GetUserOrgs(ctx context.Context, memberID string) ([]*registryv1.User, error) {
	rows, err := s.db.Query(ctx, `
SELECT`+userSelectColumnsQualified+`
FROM users u
JOIN org_memberships om ON u.id = om.org_id
WHERE om.member_id = $1
ORDER BY u.username`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*registryv1.User
	for rows.Next() {
		user := &registryv1.User{}
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&user.Id, &createTime, &updateTime,
			&user.Username, &user.Email, &user.Password,
			&user.Type, &user.State, &user.Description, &user.Url,
		); err != nil {
			return nil, err
		}
		user.CreateTime = timestamppb.New(createTime)
		user.UpdateTime = timestamppb.New(updateTime)
		orgs = append(orgs, user)
	}
	return orgs, rows.Err()
}

// CountMembers returns the number of members in the given org.
func (s *OrgStorage) CountMembers(ctx context.Context, orgID string) (int32, error) {
	var count int32
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM org_memberships WHERE org_id = $1`, orgID,
	).Scan(&count)
	return count, err
}

// GetMemberRole returns the role of memberID in orgID, or "" if not a member.
func (s *OrgStorage) GetMemberRole(ctx context.Context, orgID, memberID string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx,
		`SELECT role FROM org_memberships WHERE org_id=$1 AND member_id=$2`,
		orgID, memberID,
	).Scan(&role)
	if err != nil {
		// pgx.ErrNoRows means not a member - return empty string, no error
		return "", nil
	}
	return role, nil
}

// ListMembers returns all members of the organization identified by orgID
// together with each member's role, ordered alphabetically by username.
func (s *OrgStorage) ListMembers(ctx context.Context, orgID string) ([]*OrgMember, error) {
	query := `
SELECT` + userSelectColumnsQualified + `, om.role
FROM users u
JOIN org_memberships om ON u.id = om.member_id
WHERE om.org_id = $1
ORDER BY u.username`

	rows, err := s.db.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*OrgMember
	for rows.Next() {
		user := &registryv1.User{}
		var createTime, updateTime time.Time
		var role string
		if err := rows.Scan(
			&user.Id,
			&createTime,
			&updateTime,
			&user.Username,
			&user.Email,
			&user.Password,
			&user.Type,
			&user.State,
			&user.Description,
			&user.Url,
			&role,
		); err != nil {
			return nil, err
		}
		user.CreateTime = timestamppb.New(createTime)
		user.UpdateTime = timestamppb.New(updateTime)
		members = append(members, &OrgMember{User: user, Role: role})
	}
	return members, rows.Err()
}
