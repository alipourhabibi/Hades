// Package resource defines the storage interface for the resource registry -
// the central lookup table that maps every entity UUID to its resource type.
package resource

import "context"

// ResourceType identifies what kind of entity a UUID refers to.
type ResourceType string

const (
	ResourceTypeModule ResourceType = "module"
	ResourceTypeCommit ResourceType = "commit"
	ResourceTypeLabel  ResourceType = "label"
)

// Storage is the domain interface for the resources table.
type Storage interface {
	// ResolveType returns the ResourceType for the given entity ID.
	// Returns a NotFound error if the ID is not registered.
	ResolveType(ctx context.Context, id string) (ResourceType, error)

	// Register inserts a (id, resource_type) row.
	// Called within the same transaction as the entity insert.
	// ON CONFLICT DO NOTHING so duplicate calls are safe.
	Register(ctx context.Context, id string, rt ResourceType) error
}
