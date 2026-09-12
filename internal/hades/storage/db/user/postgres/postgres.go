// Package postgres provides the PostgreSQL implementation of user.Storage.
package postgres

import (
	"context"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserStorage implements user.Storage against a PostgreSQL database.
type UserStorage struct {
	pool *pgxpool.Pool
}

// New creates a UserStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *UserStorage {
	return &UserStorage{pool: pool}
}

var _ user.Storage = (*UserStorage)(nil)

// q returns the active querier: the transaction from context if inside a UoW,
// otherwise the connection pool.
func (u *UserStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return u.pool
}

// GetByUsername returns the user with the given username.
func (u *UserStorage) GetByUsername(ctx context.Context, username string) (*identityv1.User, error) {
	query := `
SELECT
  id,
  create_time,
  update_time,
  username,
  email,
  password,
  type,
  state,
  description,
  url
FROM users
WHERE username = $1`

	usr := &identityv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, username).Scan(
		&usr.Id,
		&createTime,
		&updateTime,
		&usr.Username,
		&usr.Email,
		&usr.Password,
		&usr.Type,
		&usr.State,
		&usr.Description,
		&usr.Url,
	)
	if err != nil {
		return nil, err
	}
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	return usr, nil
}

// GetByID returns the user with the given UUID.
func (u *UserStorage) GetByID(ctx context.Context, id string) (*identityv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE id = $1`

	usr := &identityv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, id).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	if err != nil {
		return nil, err
	}
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	return usr, nil
}

// GetByEmail returns the user with the given email address.
func (u *UserStorage) GetByEmail(ctx context.Context, email string) (*identityv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE email = $1`

	usr := &identityv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, email).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	if err != nil {
		return nil, err
	}
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	return usr, nil
}

// IncrementFailedLogins atomically increments the failed_login_count counter.
func (u *UserStorage) IncrementFailedLogins(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET failed_login_count = failed_login_count + 1, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// ResetFailedLogins resets the failed_login_count to zero.
func (u *UserStorage) ResetFailedLogins(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET failed_login_count = 0, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// LockUntil sets the locked_until timestamp.
func (u *UserStorage) LockUntil(ctx context.Context, userID string, until time.Time) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET locked_until = $1, update_time = NOW() WHERE id = $2`,
		until, userID,
	)
	return err
}

// SetEmailVerified sets email_verified_at to the current time.
func (u *UserStorage) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET email_verified_at = NOW(), update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// UpdatePassword replaces the hashed password for a user.
func (u *UserStorage) UpdatePassword(ctx context.Context, userID, newHash string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET password = $1, update_time = NOW() WHERE id = $2`,
		newHash, userID,
	)
	return err
}

// GetAuthFieldsByUsername returns auth-related fields for the given username.
func (u *UserStorage) GetAuthFieldsByUsername(ctx context.Context, username string) (*user.AuthFields, error) {
	query := `
SELECT id, username, email, password,
       email_verified_at, COALESCE(failed_login_count,0), locked_until
FROM users WHERE username = $1`
	af := &user.AuthFields{}
	err := u.q(ctx).QueryRow(ctx, query, username).Scan(
		&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&af.EmailVerifiedAt, &af.FailedLoginCount, &af.LockedUntil,
	)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// GetAuthFieldsByID returns auth-related fields for the given user UUID.
func (u *UserStorage) GetAuthFieldsByID(ctx context.Context, id string) (*user.AuthFields, error) {
	query := `
SELECT id, username, email, password,
       email_verified_at, COALESCE(failed_login_count,0), locked_until
FROM users WHERE id = $1`
	af := &user.AuthFields{}
	err := u.q(ctx).QueryRow(ctx, query, id).Scan(
		&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&af.EmailVerifiedAt, &af.FailedLoginCount, &af.LockedUntil,
	)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// Create inserts a new user row.
func (u *UserStorage) Create(
	ctx context.Context,
	username, email, password string,
	t identityv1.UserType,
	status identityv1.UserState,
	description, url string,
) error {
	query := `
INSERT INTO users (
  username,
  email,
  password,
  type,
  state,
  description,
  url
) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := u.q(ctx).Exec(ctx, query,
		username, email, password, t, status, description, url,
	)
	return err
}

// List returns users (type=USER_TYPE_USER) whose username contains query (case-insensitive).
func (u *UserStorage) List(ctx context.Context, query string) ([]*identityv1.User, error) {
	rows, err := u.q(ctx).Query(ctx, `
SELECT id, create_time, update_time, username, email, password, type, state, description, url
FROM users
WHERE type = 2
  AND ($1 = '' OR username ILIKE '%' || $1 || '%')
ORDER BY username
LIMIT 50`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*identityv1.User
	for rows.Next() {
		usr := &identityv1.User{}
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&usr.Id, &createTime, &updateTime,
			&usr.Username, &usr.Email, &usr.Password,
			&usr.Type, &usr.State, &usr.Description, &usr.Url,
		); err != nil {
			return nil, err
		}
		usr.CreateTime = timestamppb.New(createTime)
		usr.UpdateTime = timestamppb.New(updateTime)
		users = append(users, usr)
	}
	return users, rows.Err()
}

// Update sets description and url for the given user and returns the updated row.
func (u *UserStorage) Update(ctx context.Context, userID, description, url string) (*identityv1.User, error) {
	usr := &identityv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, `
UPDATE users SET description=$1, url=$2, update_time=NOW()
WHERE id=$3
RETURNING id, create_time, update_time, username, email, password, type, state, description, url`,
		description, url, userID,
	).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	if err != nil {
		return nil, err
	}
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	return usr, nil
}
