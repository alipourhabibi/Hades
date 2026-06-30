// Package org provides storage operations for organization users and membership records.
package org

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// OrgMember holds a user and their role in an org.
type OrgMember struct {
	User *registryv1.User
	Role string
}

// Storage is the domain interface for organization persistence.
type Storage interface {
	GetByName(ctx context.Context, name string) (*registryv1.User, error)
	List(ctx context.Context, query string) ([]*registryv1.User, error)
	Create(ctx context.Context, name, description, url, creatorID string) (*registryv1.User, error)
	Update(ctx context.Context, orgID, description, url string) (*registryv1.User, error)
	AddMember(ctx context.Context, orgID, memberID, role string) error
	RemoveMember(ctx context.Context, orgID, memberID string) error
	GetUserOrgs(ctx context.Context, memberID string) ([]*registryv1.User, error)
	CountMembers(ctx context.Context, orgID string) (int32, error)
	GetMemberRole(ctx context.Context, orgID, memberID string) (string, error)
	ListMembers(ctx context.Context, orgID string) ([]*OrgMember, error)
}
