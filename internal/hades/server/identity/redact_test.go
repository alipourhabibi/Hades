package identity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
)

func TestRedactEmails_KeepsOnlyTheCallersOwn(t *testing.T) {
	caller := &identityv1.User{Id: "u-alice", Email: "alice@example.com"}
	other := &identityv1.User{Id: "u-bob", Email: "bob@example.com"}

	redactEmails(caller, caller, other)

	assert.Equal(t, "alice@example.com", caller.Email, "the caller reads their own address on the settings page")
	assert.Empty(t, other.Email)
}

func TestRedactEmails_AnonymousCallerSeesNone(t *testing.T) {
	// GetUser is readable without a session, so a nil caller must not match any
	// record. An empty caller id compared against an empty user id would.
	users := []*identityv1.User{
		{Id: "u-alice", Email: "alice@example.com"},
		{Id: "", Email: "orphan@example.com"},
	}

	redactEmails(nil, users...)

	assert.Empty(t, users[0].Email)
	assert.Empty(t, users[1].Email)
}

func TestRedactEmails_ToleratesNilRecords(t *testing.T) {
	assert.NotPanics(t, func() {
		redactEmails(&identityv1.User{Id: "u-alice"}, nil, nil)
	})
}

func TestCallerFrom(t *testing.T) {
	assert.Nil(t, callerFrom(context.Background()))

	user := &identityv1.User{Id: "u-alice"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyUser, user)
	assert.Same(t, user, callerFrom(ctx))
}
