// Package user provides PostgreSQL storage for user accounts, authentication
// fields, and session-based lookups.
package user

import (
	"context"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, allowing storage
// methods to run either against a connection pool or inside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// UserStorage executes user queries against PostgreSQL. It supports both
// pooled connections and explicit transactions via WithTx.
type UserStorage struct {
	db querier
}

// New returns a UserStorage backed by the given connection pool.
func New(pool *pgxpool.Pool) *UserStorage {
	return &UserStorage{
		db: pool,
	}
}

// WithTx returns a copy of UserStorage bound to the given transaction.
func (u *UserStorage) WithTx(tx pgx.Tx) *UserStorage {
	return &UserStorage{db: tx}
}

// GetByUsername returns the full user row matching the given username.
// Returns pgx.ErrNoRows when no match exists.
func (u *UserStorage) GetByUsername(ctx context.Context, username string) (*registryv1.User, error) {
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

	user := &registryv1.User{}
	var createTime time.Time
	var updateTime time.Time
	err := u.db.QueryRow(ctx, query, username).Scan(
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
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetByID returns the user with the given UUID.
func (u *UserStorage) GetByID(ctx context.Context, id string) (*registryv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE id = $1`

	user := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.db.QueryRow(ctx, query, id).Scan(
		&user.Id, &createTime, &updateTime,
		&user.Username, &user.Email, &user.Password,
		&user.Type, &user.State, &user.Description, &user.Url,
	)
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetByEmail returns the user with the given email address.
func (u *UserStorage) GetByEmail(ctx context.Context, email string) (*registryv1.User, error) {
	query := `
SELECT
  id, create_time, update_time,
  username, email, password,
  type, state, description, url
FROM users
WHERE email = $1`

	user := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.db.QueryRow(ctx, query, email).Scan(
		&user.Id, &createTime, &updateTime,
		&user.Username, &user.Email, &user.Password,
		&user.Type, &user.State, &user.Description, &user.Url,
	)
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// IncrementFailedLogins atomically increments the failed_login_count counter.
func (u *UserStorage) IncrementFailedLogins(ctx context.Context, userID string) error {
	_, err := u.db.Exec(ctx,
		`UPDATE users SET failed_login_count = failed_login_count + 1, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// ResetFailedLogins resets the failed_login_count to zero.
func (u *UserStorage) ResetFailedLogins(ctx context.Context, userID string) error {
	_, err := u.db.Exec(ctx,
		`UPDATE users SET failed_login_count = 0, update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// LockUntil sets the locked_until timestamp, preventing login until that time elapses.
func (u *UserStorage) LockUntil(ctx context.Context, userID string, until time.Time) error {
	_, err := u.db.Exec(ctx,
		`UPDATE users SET locked_until = $1, update_time = NOW() WHERE id = $2`,
		until, userID,
	)
	return err
}

// SetEmailVerified sets email_verified_at to the current time.
func (u *UserStorage) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := u.db.Exec(ctx,
		`UPDATE users SET email_verified_at = NOW(), update_time = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// UpdatePassword replaces the hashed password for a user.
func (u *UserStorage) UpdatePassword(ctx context.Context, userID, newHash string) error {
	_, err := u.db.Exec(ctx,
		`UPDATE users SET password = $1, update_time = NOW() WHERE id = $2`,
		newHash, userID,
	)
	return err
}

// AuthFields holds fields required during login / password validation.
type AuthFields struct {
	ID               string
	Username         string
	Email            string
	PasswordHash     string
	EmailVerifiedAt  *time.Time
	FailedLoginCount int
	LockedUntil      *time.Time
}

// GetAuthFieldsByUsername returns auth-related fields for the given username.
func (u *UserStorage) GetAuthFieldsByUsername(ctx context.Context, username string) (*AuthFields, error) {
	query := `
SELECT id, username, email, password,
       email_verified_at, COALESCE(failed_login_count,0), locked_until
FROM users WHERE username = $1`
	af := &AuthFields{}
	err := u.db.QueryRow(ctx, query, username).Scan(
		&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&af.EmailVerifiedAt, &af.FailedLoginCount, &af.LockedUntil,
	)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// GetAuthFieldsByID returns auth-related fields for the given user UUID.
func (u *UserStorage) GetAuthFieldsByID(ctx context.Context, id string) (*AuthFields, error) {
	query := `
SELECT id, username, email, password,
       email_verified_at, COALESCE(failed_login_count,0), locked_until
FROM users WHERE id = $1`
	af := &AuthFields{}
	err := u.db.QueryRow(ctx, query, id).Scan(
		&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&af.EmailVerifiedAt, &af.FailedLoginCount, &af.LockedUntil,
	)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// Create inserts a new user row. The password must already be hashed by the caller.
func (u *UserStorage) Create(
	ctx context.Context,
	username string,
	email string,
	password string,
	t registryv1.UserType,
	status registryv1.UserState,
	description string,
	url string,

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
) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := u.db.Exec(ctx, query,
		username,
		email,
		password,
		t,
		status,
		description,
		url,
	)

	return err
}

// List returns users (type=USER_TYPE_USER) whose username contains query
// (case-insensitive).  If query is empty the first 50 users are returned.
func (u *UserStorage) List(ctx context.Context, query string) ([]*registryv1.User, error) {
	rows, err := u.db.Query(ctx, `
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
		users = append(users, user)
	}
	return users, rows.Err()
}

// Update sets description and url for the given user and returns the updated row.
func (u *UserStorage) Update(ctx context.Context, userID, description, url string) (*registryv1.User, error) {
	user := &registryv1.User{}
	var createTime, updateTime time.Time
	err := u.db.QueryRow(ctx, `
UPDATE users SET description=$1, url=$2, update_time=NOW()
WHERE id=$3
RETURNING id, create_time, update_time, username, email, password, type, state, description, url`,
		description, url, userID,
	).Scan(
		&user.Id, &createTime, &updateTime,
		&user.Username, &user.Email, &user.Password,
		&user.Type, &user.State, &user.Description, &user.Url,
	)
	if err != nil {
		return nil, err
	}
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	return user, nil
}

// GetBySessionId returns the user associated with a non-expired session.
// Expired or unknown session IDs produce pgx.ErrNoRows.
func (u *UserStorage) GetBySessionId(ctx context.Context, sessionId string) (*registryv1.User, error) {
	query := `
SELECT
  u.id,
  u.create_time,
  u.update_time,
  u.username,
  u.email,
  u.type,
  u.state,
  u.description,
  u.url
FROM users u
WHERE u.id = (
  SELECT user_id FROM sessions
  WHERE id = $1 AND expires_at > NOW()
)
`

	user := &registryv1.User{}
	var createTime time.Time
	var updateTime time.Time
	err := u.db.QueryRow(ctx, query, sessionId).Scan(
		&user.Id,
		&createTime,
		&updateTime,
		&user.Username,
		&user.Email,
		&user.Type,
		&user.State,
		&user.Description,
		&user.Url,
	)
	user.CreateTime = timestamppb.New(createTime)
	user.UpdateTime = timestamppb.New(updateTime)
	if err != nil {
		return nil, err
	}

	return user, nil
}
