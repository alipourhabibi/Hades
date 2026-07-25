// Package postgres provides PostgreSQL-backed implementations of all storage interfaces.
package postgres

import (
	"context"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	pkgerrors "github.com/alipourhabibi/Hades/internal/errors"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserStorage executes user queries against PostgreSQL.
type UserStorage struct {
	pool *pgxpool.Pool
}

// NewUser returns a UserStorage backed by the given connection pool.
func NewUser(pool *pgxpool.Pool) *UserStorage {
	return &UserStorage{pool: pool}
}

// q returns the active transaction from ctx, or falls back to the pool.
func (u *UserStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return u.pool
}

func (u *UserStorage) GetByUsername(ctx context.Context, username string) (*registryv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE username = $1`

	usr := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, username).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}
	return usr, nil
}

func (u *UserStorage) GetByID(ctx context.Context, id string) (*registryv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE id = $1`

	usr := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, id).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}
	return usr, nil
}

func (u *UserStorage) GetByEmail(ctx context.Context, email string) (*registryv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE email = $1`

	usr := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, email).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email, &usr.Password,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}
	return usr, nil
}

func (u *UserStorage) IncrementFailedLogins(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET failed_login_count = failed_login_count + 1, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (u *UserStorage) ResetFailedLogins(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET failed_login_count = 0, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (u *UserStorage) LockUntil(ctx context.Context, userID string, until time.Time) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET locked_until = $1, update_time = NOW() WHERE id = $2`,
		until, userID,
	)
	return err
}

func (u *UserStorage) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET email_verified_at = NOW(), update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (u *UserStorage) UpdatePassword(ctx context.Context, userID, newHash string) error {
	_, err := u.q(ctx).Exec(ctx,
		`UPDATE users SET password = $1, update_time = NOW() WHERE id = $2`,
		newHash, userID,
	)
	return err
}

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

func (u *UserStorage) Create(
	ctx context.Context,
	username, email, password string,
	t registryv1.UserType,
	status registryv1.UserState,
	description, url string,
) error {
	query := `
INSERT INTO users (
  username, email, password, type, state, description, url
) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := u.q(ctx).Exec(ctx, query, username, email, password, t, status, description, url)
	return err
}

func (u *UserStorage) List(ctx context.Context, query string) ([]*registryv1.User, error) {
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

	var users []*registryv1.User
	for rows.Next() {
		usr := &registryv1.User{}
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

func (u *UserStorage) Update(ctx context.Context, userID, description, url string) (*registryv1.User, error) {
	usr := &registryv1.User{}
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

func (u *UserStorage) GetBySessionId(ctx context.Context, sessionId string) (*registryv1.User, error) {
	query := `
SELECT
  u.id, u.create_time, u.update_time,
  u.username, u.email, u.type, u.state, u.description, u.url
FROM users u
WHERE u.id = (
  SELECT user_id FROM sessions
  WHERE token_hash = $1 AND expires_at > NOW()
)`

	usr := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.q(ctx).QueryRow(ctx, query, sessionId).Scan(
		&usr.Id, &createTime, &updateTime,
		&usr.Username, &usr.Email,
		&usr.Type, &usr.State, &usr.Description, &usr.Url,
	)
	usr.CreateTime = timestamppb.New(createTime)
	usr.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, pkgerrors.FromPgx(err)
	}
	return usr, nil
}

var _ user.Storage = (*UserStorage)(nil)
