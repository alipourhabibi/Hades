// Package commit defines the storage interface for commit metadata.
package commit

import (
	"context"
	"encoding/hex"
	"fmt"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/google/uuid"

	"github.com/alipourhabibi/Hades/utils/connerr"
)

// ErrNotFound is returned when no commit matches the lookup.
//
// It is a sentinel rather than a per-backend error string: SQLite returned
// fmt.Errorf("commit not found"), which the handler boundary translated to
// Internal, while PostgreSQL returned NotFound. The same call produced a
// different HTTP status depending on which backend was configured.
var ErrNotFound = fmt.Errorf("commit: %w", connerr.ErrNotFound)

// DecodeDigest converts the hex digest text stored in commits.digest_value
// into the raw bytes registryv1.Digest.Value is declared to hold.
//
// The column is hex text, which is worth keeping: it is what a human reads in
// a git commit message and in psql, and GetCommitByDigest looks up by it. The
// proto field is bytes, and buf enforces the length: a b5 digest is 64 bytes,
// and a hex rendering of one is 128 characters. Assigning the column straight
// to the field, which is what every scanner used to do, produced a 128 byte
// value whose contents were the characters "c", "3", "e" and so on. buf
// rejected it with "invalid shake256 digest value: expected 64 bytes, got
// 128", so no module could be a dependency of another.
//
// An undecodable value returns nil rather than the raw text. Emitting
// something a client will certainly reject is worse than emitting nothing:
// nil is visibly absent, while wrong-length bytes look like data.
//
// The parameter is a pointer because the column is nullable: a commit written
// before the digest existed has NULL here, and scanning that into a string
// fails on both drivers.
func DecodeDigest(hexValue *string) []byte {
	if hexValue == nil || *hexValue == "" {
		return nil
	}
	raw, err := hex.DecodeString(*hexValue)
	if err != nil {
		return nil
	}
	return raw
}

// Storage is the domain interface for commit persistence.
type Storage interface {
	Create(ctx context.Context, id uuid.UUID, commitHash, ownerId, moduleId string, digestType registryv1.DigestType, digestValue, createdByUserId, sourceControlUrl string) error
	GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error)
	// GetCommitByDigest looks up a commit by digest value + module ID for dedup checks.
	// Returns (nil, nil) when no matching commit exists.
	GetCommitByDigest(ctx context.Context, moduleID, digestValue string) (*registryv1.Commit, error)
	GetCommitByOwnerModule(ctx context.Context, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
	ListByModule(ctx context.Context, moduleID string, limit, offset int) ([]*registryv1.Commit, error)
	GetByHash(ctx context.Context, commitHash string) (*registryv1.Commit, error)
	GetByHashPrefix(ctx context.Context, prefix string) (*registryv1.Commit, error)
	DeleteByIds(ctx context.Context, ids []string) error
}
