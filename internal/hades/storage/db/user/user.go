package user

import (
	"context"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserStorage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *UserStorage {
	return &UserStorage{
		pool: pool,
	}
}

// GetByUsername will get user by its username
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
	err := u.pool.QueryRow(ctx, query, username).Scan(
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

// Create will create user
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

	_, err := u.pool.Exec(ctx, query,
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

// GetBySessionId will get the user by sessionId and expires_at check
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
	err := u.pool.QueryRow(ctx, query, sessionId).Scan(
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
