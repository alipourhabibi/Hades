// Package sqlite provides SQLite-backed implementations of all storage interfaces.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"connectrpc.com/connect"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SQLiteUserStorage implements user.Storage using database/sql with SQLite.
type SQLiteUserStorage struct {
	db *sql.DB
}

func NewUser(db *sql.DB) *SQLiteUserStorage {
	return &SQLiteUserStorage{db: db}
}

func (s *SQLiteUserStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func scanSQLiteUser(row *sql.Row) (*identityv1.User, error) {
	u := &identityv1.User{}
	var createTime, updateTime sqltypes.Time
	var password sql.NullString
	err := row.Scan(
		&u.Id, &createTime, &updateTime,
		&u.Username, &u.Email, &password,
		&u.Type, &u.State, &u.Description, &u.Url,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, err
	}
	u.Password = password.String
	u.CreateTime = timestamppb.New(createTime.V)
	u.UpdateTime = timestamppb.New(updateTime.V)
	return u, nil
}

const sqliteUserColumns = `id, create_time, update_time, username, email, password, type, state, description, url`

func (s *SQLiteUserStorage) GetByUsername(ctx context.Context, username string) (*identityv1.User, error) {
	return scanSQLiteUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserColumns+` FROM users WHERE username = ?`, username))
}

func (s *SQLiteUserStorage) GetByID(ctx context.Context, id string) (*identityv1.User, error) {
	return scanSQLiteUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserColumns+` FROM users WHERE id = ?`, id))
}

func (s *SQLiteUserStorage) GetByEmail(ctx context.Context, email string) (*identityv1.User, error) {
	return scanSQLiteUser(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteUserColumns+` FROM users WHERE email = ?`, email))
}

func (s *SQLiteUserStorage) GetBySessionId(ctx context.Context, sessionId string) (*identityv1.User, error) {
	return scanSQLiteUser(s.q(ctx).QueryRowContext(ctx, `
SELECT `+sqliteUserColumns+`
FROM users
WHERE id = (
  SELECT user_id FROM sessions
  WHERE token_hash = ? AND expires_at > datetime('now')
)`, sessionId))
}

func (s *SQLiteUserStorage) GetAuthFieldsByUsername(ctx context.Context, username string) (*user.AuthFields, error) {
	af := &user.AuthFields{}
	var emailVerifiedAt, lockedUntil sqltypes.NullTime
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, username, email, password,
		        email_verified_at, COALESCE(failed_login_count,0), locked_until
		 FROM users WHERE username = ?`, username,
	).Scan(&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&emailVerifiedAt, &af.FailedLoginCount, &lockedUntil)
	if err != nil {
		return nil, err
	}
	af.EmailVerifiedAt = emailVerifiedAt.Ptr()
	af.LockedUntil = lockedUntil.Ptr()
	return af, nil
}

func (s *SQLiteUserStorage) GetAuthFieldsByID(ctx context.Context, id string) (*user.AuthFields, error) {
	af := &user.AuthFields{}
	var emailVerifiedAt, lockedUntil sqltypes.NullTime
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, username, email, password,
		        email_verified_at, COALESCE(failed_login_count,0), locked_until
		 FROM users WHERE id = ?`, id,
	).Scan(&af.ID, &af.Username, &af.Email, &af.PasswordHash,
		&emailVerifiedAt, &af.FailedLoginCount, &lockedUntil)
	if err != nil {
		return nil, err
	}
	af.EmailVerifiedAt = emailVerifiedAt.Ptr()
	af.LockedUntil = lockedUntil.Ptr()
	return af, nil
}

func (s *SQLiteUserStorage) Create(ctx context.Context, username, email, password string, t identityv1.UserType, status identityv1.UserState, description, url string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO users (username, email, password, type, state, description, url)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username, email, password, t, status, description, url,
	)
	return err
}

func (s *SQLiteUserStorage) List(ctx context.Context, query string) ([]*identityv1.User, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteUserColumns+`
		 FROM users
		 WHERE type = 2
		   AND (? = '' OR username LIKE '%' || ? || '%')
		 ORDER BY username LIMIT 50`, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteUsers(rows)
}

func scanSQLiteUsers(rows *sql.Rows) ([]*identityv1.User, error) {
	var users []*identityv1.User
	for rows.Next() {
		u := &identityv1.User{}
		var createTime, updateTime sqltypes.Time
		var password sql.NullString
		if err := rows.Scan(
			&u.Id, &createTime, &updateTime,
			&u.Username, &u.Email, &password,
			&u.Type, &u.State, &u.Description, &u.Url,
		); err != nil {
			return nil, err
		}
		u.Password = password.String
		u.CreateTime = timestamppb.New(createTime.V)
		u.UpdateTime = timestamppb.New(updateTime.V)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *SQLiteUserStorage) Update(ctx context.Context, userID, description, url string) (*identityv1.User, error) {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET description=?, url=?, update_time=datetime('now') WHERE id=?`,
		description, url, userID,
	)
	if err != nil {
		return nil, err
	}
	return s.GetByID(ctx, userID)
}

func (s *SQLiteUserStorage) IncrementFailedLogins(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET failed_login_count = failed_login_count + 1, update_time = datetime('now') WHERE id = ?`, userID)
	return err
}

func (s *SQLiteUserStorage) ResetFailedLogins(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET failed_login_count = 0, update_time = datetime('now') WHERE id = ?`, userID)
	return err
}

func (s *SQLiteUserStorage) LockUntil(ctx context.Context, userID string, until time.Time) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET locked_until = ?, update_time = datetime('now') WHERE id = ?`, until, userID)
	return err
}

func (s *SQLiteUserStorage) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET email_verified_at = datetime('now'), update_time = datetime('now') WHERE id = ?`, userID)
	return err
}

func (s *SQLiteUserStorage) UpdatePassword(ctx context.Context, userID, newHash string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE users SET password = ?, update_time = datetime('now') WHERE id = ?`, newHash, userID)
	return err
}

var _ user.Storage = (*SQLiteUserStorage)(nil)
