package auth

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alipourhabibi/Hades/internal/hades/constants"
)

func TestValidateScopes_RejectsEmptyList(t *testing.T) {
	// Empty means unrestricted in the enforcement path, so a client that simply
	// omits the field must not silently receive a full-authority credential.
	err := validateScopes(nil)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	assert.Error(t, validateScopes([]string{}))
}

func TestValidateScopes_RejectsUnknownScope(t *testing.T) {
	err := validateScopes([]string{"module:read", "module:teleport"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module:teleport")
}

func TestValidateScopes_RejectsDuplicates(t *testing.T) {
	err := validateScopes([]string{"module:read", "module:read"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateScopes_AcceptsKnownScopes(t *testing.T) {
	assert.NoError(t, validateScopes([]string{
		constants.Scope(constants.ResourceModule, constants.ActionRead),
		constants.Scope(constants.ResourceModule, constants.ActionPush),
	}))
	assert.NoError(t, validateScopes([]string{"module:*"}))
}

func TestIsKnownScope(t *testing.T) {
	assert.True(t, constants.IsKnownScope("module:read"))
	assert.True(t, constants.IsKnownScope("module:*"))
	// Only "module" is a scopable resource. The other policy resource types are
	// never asked about by any authorization check, so a token scoped to one
	// could do nothing at all; refusing it at creation beats issuing a dead
	// credential. See constants.scopedResources.
	assert.False(t, constants.IsKnownScope("commit:read"))
	assert.False(t, constants.IsKnownScope("label:create"))
	assert.False(t, constants.IsKnownScope("namespace:admin"))
	assert.False(t, constants.IsKnownScope("module:"))
	assert.False(t, constants.IsKnownScope("*"))
	assert.False(t, constants.IsKnownScope(""))
	assert.False(t, constants.IsKnownScope("admin"))
}

func TestIsReservedName(t *testing.T) {
	// Users and organisations share one namespace, and the name becomes the
	// first path segment of every module, so both must be checked.
	assert.True(t, constants.IsReservedName("go"))
	assert.True(t, constants.IsReservedName("GO"))
	assert.True(t, constants.IsReservedName(" settings "))
	assert.True(t, constants.IsReservedName("gen"))
	assert.False(t, constants.IsReservedName("alice"))
}
