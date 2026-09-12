// Package auditlog provides an append-only audit log for security-relevant user actions.
package auditlog

import (
	"context"
	"time"

	authv1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	"github.com/google/uuid"
)

// Page size bounds for List. The upper clamp matches every other paginated
// method in the storage layer; without it a caller could request an arbitrary
// number of security-event rows in one call.
const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// Storage is the domain interface for audit log persistence.
type Storage interface {
	Create(ctx context.Context, userID *string, event authv1.AuditEventType, ipAddress, userAgent string, metadata map[string]any) error
	List(ctx context.Context, userID string, pageSize, offset int) ([]*Row, error)
	RecentIPsForUser(ctx context.Context, userID string, n int) ([]string, error)
}

type Row struct {
	ID        uuid.UUID
	UserID    *string
	EventType authv1.AuditEventType
	IPAddress string
	UserAgent string
	Metadata  map[string]any
	CreatedAt time.Time
}
