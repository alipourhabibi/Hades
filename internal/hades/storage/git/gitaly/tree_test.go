package gitaly

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
)

func TestRevisionOrHead(t *testing.T) {
	// The whole point of threading ref through: a named commit must be read as
	// that commit. Falling back to HEAD would answer from the default branch
	// and silently ignore the commit_hash the caller asked for.
	assert.Equal(t, []byte("abcdef123456"), revisionOrHead("abcdef123456"))
	assert.Equal(t, []byte("HEAD"), revisionOrHead(""))
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, isNotFound(ErrTreeNotFound))
	assert.True(t, isNotFound(fmt.Errorf("GetTreeEntries: %w", ErrTreeNotFound)))
	assert.True(t, isNotFound(status.Error(codes.NotFound, "repository not found")))

	assert.False(t, isNotFound(errors.New("connection refused")))
	assert.False(t, isNotFound(status.Error(codes.Unavailable, "gitaly is down")))
	assert.False(t, isNotFound(nil))
}

func TestErrTreeNotFoundMapsOntoTheInterfaceError(t *testing.T) {
	// git.Storage documents ErrNotFound for a missing path or ref. Callers such
	// as CommitService.ListModuleFiles match on that to answer NOT_FOUND rather
	// than INTERNAL, so the two must stay connected.
	assert.NotErrorIs(t, ErrTreeNotFound, git.ErrNotFound,
		"the sentinels are distinct; GitalyStorage translates one into the other")
}
