// Package org provides storage operations for organization users and membership records.
package org

import (
	"context"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
)

// OrgMember holds a user and their role in an org.
type OrgMember struct {
	User *identityv1.User
	Role string
}

// Storage is the domain interface for organization persistence.
type Storage interface {
	GetByName(ctx context.Context, name string) (*identityv1.User, error)
	// List returns organisations whose username contains query, at most limit of
	// them starting at offset. The query term is matched literally.
	List(ctx context.Context, query string, limit, offset int) ([]*identityv1.User, error)
	Create(ctx context.Context, name, description, url, creatorID string) (*identityv1.User, error)
	Update(ctx context.Context, orgID, description, url string) (*identityv1.User, error)
	AddMember(ctx context.Context, orgID, memberID, role string) error
	RemoveMember(ctx context.Context, orgID, memberID string) error
	// GetUserOrgs returns the organisations the member belongs to, capped at
	// 1000. The cap is there so the method cannot return an unbounded set; a
	// person in a thousand organisations is an anomaly, not a page.
	GetUserOrgs(ctx context.Context, memberID string) ([]*identityv1.User, error)
	CountMembers(ctx context.Context, orgID string) (int32, error)
	// GetMemberRole returns the caller's role in the organisation.
	// Returns the driver's no-rows error when the user is not a member, so
	// callers can tell absence from a lookup that failed.
	GetMemberRole(ctx context.Context, orgID, memberID string) (string, error)
	// ListMembers returns the organisation's members, capped at 1000. See
	// GetUserOrgs for why a cap rather than a cursor.
	ListMembers(ctx context.Context, orgID string) ([]*OrgMember, error)
}
