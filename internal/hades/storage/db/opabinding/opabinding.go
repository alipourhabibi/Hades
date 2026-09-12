// Package opabinding provides PostgreSQL persistence for OPA role bindings.
package opabinding

import (
	"context"
	"time"
)

// RoleBinding is a single row from the opa_role_bindings table.
type RoleBinding struct {
	ID        string
	Subject   string
	Role      string
	Domain    string
	CreatedAt time.Time
}

// Storage is the domain interface for OPA binding persistence.
type Storage interface {
	Create(ctx context.Context, subject, role, domain string) error
	CreateBatch(ctx context.Context, bindings []RoleBinding) error
	ListBySubject(ctx context.Context, subject string) ([]RoleBinding, error)
	Delete(ctx context.Context, id string) error
	DeleteBySubjectDomain(ctx context.Context, subject, domain string) error
}
