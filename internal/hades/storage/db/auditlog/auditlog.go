// Package auditlog provides an append-only audit log for security-relevant user actions.
package auditlog

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for audit log persistence.
type Storage interface {
	Create(ctx context.Context, userID *string, event, ipAddress, userAgent string, metadata map[string]any) error
	List(ctx context.Context, userID string, pageSize, offset int) ([]*Row, error)
	RecentIPsForUser(ctx context.Context, userID string, n int) ([]string, error)
}

type Row struct {
	ID        uuid.UUID
	UserID    *string
	Event     string
	IPAddress string
	UserAgent string
	Metadata  map[string]any
	CreatedAt time.Time
}
