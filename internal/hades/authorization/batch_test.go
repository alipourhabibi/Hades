package authorization

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/opabinding"
)

// emptyBindingStore has no role bindings at all, so every policy is denied.
type emptyBindingStore struct{}

func (emptyBindingStore) Create(context.Context, string, string, string) error { return nil }
func (emptyBindingStore) ListBySubject(context.Context, string) ([]opabinding.RoleBinding, error) {
	return nil, nil
}
func (emptyBindingStore) DeleteBySubjectDomain(context.Context, string, string) error { return nil }

// ownerBindingStore grants owner over "alice/*".
type ownerBindingStore struct{}

func (ownerBindingStore) Create(context.Context, string, string, string) error { return nil }
func (ownerBindingStore) ListBySubject(_ context.Context, subject string) ([]opabinding.RoleBinding, error) {
	if subject != "alice" {
		return nil, nil
	}
	return []opabinding.RoleBinding{{Subject: "alice", Role: constants.RoleOwner, Domain: "alice/*"}}, nil
}
func (ownerBindingStore) DeleteBySubjectDomain(context.Context, string, string) error { return nil }

func pushPolicy(subject, domain string) constants.Policy {
	return constants.Policy{
		Subject:      subject,
		Domain:       domain,
		ResourceType: string(constants.ResourceModule),
		Action:       string(constants.ActionPush),
		Visibility:   constants.VisibilityPrivate,
	}
}

func TestBatchAllow_DeniesWhenNoBindingsExist(t *testing.T) {
	// The critical property: a caller with no bindings must come back denied.
	// The previous implementation pre-filled the result slice with true, so any
	// path that returned early handed back an all-allowed answer.
	ctx := context.Background()
	e, err := newFromStore(ctx, emptyBindingStore{}, cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)

	got, err := e.BatchAllow(ctx, []constants.Policy{
		pushPolicy("mallory", "alice/secret"),
		pushPolicy("mallory", "bob/secret"),
	})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.False(t, got[0], "a subject with no bindings must be denied")
	assert.False(t, got[1], "a subject with no bindings must be denied")
}

func TestBatchAllow_MatchesSinglePolicyAllow(t *testing.T) {
	// BatchAllow and Allow must never disagree: the batch path gates uploads and
	// the single path gates everything else.
	ctx := context.Background()
	e, err := newFromStore(ctx, ownerBindingStore{}, cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)

	policies := []constants.Policy{
		pushPolicy("alice", "alice/mymod"),
		pushPolicy("alice", "bob/theirs"),
		pushPolicy("mallory", "alice/mymod"),
	}

	batch, err := e.BatchAllow(ctx, policies)
	require.NoError(t, err)

	for i, p := range policies {
		single, err := e.Allow(ctx, p)
		require.NoError(t, err)
		assert.Equal(t, single, batch[i],
			"policy %d: Allow returned %v but BatchAllow returned %v", i, single, batch[i])
	}
}

func TestBatchAllow_EmptyInput(t *testing.T) {
	ctx := context.Background()
	e, err := newFromStore(ctx, emptyBindingStore{}, cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)

	got, err := e.BatchAllow(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestBatchAllow_GrantsOnlyWithinTheBoundNamespace(t *testing.T) {
	ctx := context.Background()
	e, err := newFromStore(ctx, ownerBindingStore{}, cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)

	got, err := e.BatchAllow(ctx, []constants.Policy{
		pushPolicy("alice", "alice/one"),
		pushPolicy("alice", "alice/two"),
		pushPolicy("alice", "carol/three"),
	})

	require.NoError(t, err)
	assert.True(t, got[0])
	assert.True(t, got[1])
	assert.False(t, got[2], "the owner binding covers alice/* only")
}
