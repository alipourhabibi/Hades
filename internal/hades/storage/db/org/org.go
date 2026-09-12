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
	List(ctx context.Context, query string) ([]*identityv1.User, error)
	Create(ctx context.Context, name, description, url, creatorID string) (*identityv1.User, error)
	Update(ctx context.Context, orgID, description, url string) (*identityv1.User, error)
	AddMember(ctx context.Context, orgID, memberID, role string) error
	RemoveMember(ctx context.Context, orgID, memberID string) error
	GetUserOrgs(ctx context.Context, memberID string) ([]*identityv1.User, error)
	CountMembers(ctx context.Context, orgID string) (int32, error)
	// GetMemberRole returns the caller's role in the organisation.
	// Returns the driver's no-rows error when the user is not a member, so
	// callers can tell absence from a lookup that failed.
	GetMemberRole(ctx context.Context, orgID, memberID string) (string, error)
	ListMembers(ctx context.Context, orgID string) ([]*OrgMember, error)
}
