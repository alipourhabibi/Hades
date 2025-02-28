package commit

import (
	"context"
	"errors"
	"fmt"

	"github.com/alipourhabibi/Hades/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommitStorage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *CommitStorage {
	return &CommitStorage{
		pool: pool,
	}
}

func (c *CommitStorage) GetCommitById(ctx context.Context, id string) (*models.Commit, error) {
	query := `
		SELECT 
			id,
			commit_hash,
			create_time,
			update_time,
			owner_id,
			module_id,
			digest_type,
			digest_value,
			created_by_user_id,
			source_control_url
		FROM commits
		WHERE id = $1
	`

	commit := &models.Commit{}
	err := c.pool.QueryRow(ctx, query, id).Scan(
		&commit.ID,
		&commit.CommitHash,
		&commit.CreateTime,
		&commit.UpdateTime,
		&commit.OwnerID,
		&commit.ModuleID,
		&commit.DigestType,
		&commit.DigestValue,
		&commit.CreatedByUserID,
		&commit.SourceControlURL,
	)
	if err != nil {
		return nil, err
	}
	return commit, nil
}

func (c *CommitStorage) GetCommitByQuery(ctx context.Context, query map[string]any) (*models.Commit, error) {
	baseQuery := `
		SELECT 
			id,
			commit_hash,
			create_time,
			update_time,
			owner_id,
			module_id,
			digest_type,
			digest_value,
			created_by_user_id,
			source_control_url
		FROM commits
	`

	var conditions []string
	var values []interface{}
	i := 1

	for key, value := range query {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", key, i))
		values = append(values, value)
		i++
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + conditions[0]
		for _, cond := range conditions[1:] {
			baseQuery += " AND " + cond
		}
	}

	commit := &models.Commit{}
	err := c.pool.QueryRow(ctx, baseQuery, values...).Scan(
		&commit.ID,
		&commit.CommitHash,
		&commit.CreateTime,
		&commit.UpdateTime,
		&commit.OwnerID,
		&commit.ModuleID,
		&commit.DigestType,
		&commit.DigestValue,
		&commit.CreatedByUserID,
		&commit.SourceControlURL,
	)

	if err != nil {
		if errors.Is(pgx.ErrNoRows, err) {
			return nil, nil
		}
		return nil, err
	}
	return commit, nil
}

// TODO think about it
func (c *CommitStorage) GetCommitByOwnerModule(ctx context.Context, moduleRefs []*models.ModuleRef) ([]*models.Commit, error) {
	baseQuery := `
		SELECT 
			c.id,
			c.commit_hash,
			c.create_time,
			c.update_time,
			c.owner_id,
			c.module_id,
			c.digest_type,
			c.digest_value,
			c.created_by_user_id,
			c.source_control_url,
			u.id AS owner_id, u.username AS owner_name,  -- Add the owner details
			m.id AS module_id, m.name AS module_name -- Add the module details
		FROM commits c
		JOIN modules m ON c.module_id = m.id
		JOIN users u ON c.owner_id = u.id  -- Assuming users table for Owner
	`

	var commits []*models.Commit

	for _, req := range moduleRefs {
		var (
			query  string
			values []interface{}
		)

		if req.Id != "" {
			query = baseQuery + " WHERE c.id = $1"
			values = append(values, req.Id)
		} else {
			query = baseQuery + " WHERE m.name = $1"
			values = append(values, req.Owner+"/"+req.Module)
		}
		query += " ORDER BY c.create_time DESC"

		rows, err := c.pool.Query(ctx, query, values...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			commit := &models.Commit{}
			var ownerID uuid.UUID
			var ownerName string
			var moduleID uuid.UUID
			var moduleName string

			// Scan the result into the Commit struct along with Owner and Module fields
			err := rows.Scan(
				&commit.ID,
				&commit.CommitHash,
				&commit.CreateTime,
				&commit.UpdateTime,
				&commit.OwnerID,
				&commit.ModuleID,
				&commit.DigestType,
				&commit.DigestValue,
				&commit.CreatedByUserID,
				&commit.SourceControlURL,
				&ownerID,    // Scan Owner ID
				&ownerName,  // Scan Owner Name
				&moduleID,   // Scan Module ID
				&moduleName, // Scan Module Name
			)
			if err != nil {
				return nil, err
			}

			// Now assign the related data (Owner and Module) to Commit struct
			commit.Owner = models.User{
				ID:       ownerID,
				Username: ownerName,
			}
			commit.Module = models.Module{
				ID:   moduleID,
				Name: moduleName,
			}

			commits = append(commits, commit)
		}

		// If there was an error during iteration, return it.
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return commits, nil
}
