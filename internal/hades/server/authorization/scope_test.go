package authorization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alipourhabibi/Hades/internal/hades/constants"
)

func TestScopeCovers_EmptyMeansUnrestricted(t *testing.T) {
	// Tokens issued before scopes were enforced carry no scope list. They keep
	// full authority rather than losing all access on upgrade.
	assert.True(t, scopeCovers(nil, "module", "read", "alice/mymod"))
	assert.True(t, scopeCovers([]string{}, "module", "push", "alice/mymod"))
}

func TestScopeCovers_NamespaceWideScope(t *testing.T) {
	scopes := []string{"module:read"}

	assert.True(t, scopeCovers(scopes, "module", "read", "alice/mymod"))
	assert.True(t, scopeCovers(scopes, "module", "read", "bob/other"))
	// A two-part scope has no domain restriction, so it also covers a request
	// that could not name its target.
	assert.True(t, scopeCovers(scopes, "module", "read", ""))

	assert.False(t, scopeCovers(scopes, "module", "push", "alice/mymod"))
	assert.False(t, scopeCovers(scopes, "commit", "read", "alice/mymod"))
}

func TestScopeCovers_ActionWildcard(t *testing.T) {
	scopes := []string{"module:*"}

	assert.True(t, scopeCovers(scopes, "module", "read", "alice/mymod"))
	assert.True(t, scopeCovers(scopes, "module", "delete", "alice/mymod"))
	assert.False(t, scopeCovers(scopes, "commit", "read", "alice/mymod"))
}

func TestScopeCovers_SingleModuleScope(t *testing.T) {
	// This is the case the original 2.18 note asked for: a token that can read
	// module x and nothing else.
	scopes := []string{"module:read:alice/mymod"}

	assert.True(t, scopeCovers(scopes, "module", "read", "alice/mymod"))

	assert.False(t, scopeCovers(scopes, "module", "read", "alice/other"))
	assert.False(t, scopeCovers(scopes, "module", "read", "bob/mymod"))
	assert.False(t, scopeCovers(scopes, "module", "push", "alice/mymod"))
	// A domain-restricted scope must not be stretched to cover a request whose
	// target could not be identified.
	assert.False(t, scopeCovers(scopes, "module", "read", ""))
}

func TestScopeCovers_NamespaceWildcardScope(t *testing.T) {
	scopes := []string{"module:read:alice/*"}

	assert.True(t, scopeCovers(scopes, "module", "read", "alice/one"))
	assert.True(t, scopeCovers(scopes, "module", "read", "alice/two"))

	assert.False(t, scopeCovers(scopes, "module", "read", "bob/one"))
	// "alice" alone is not inside the "alice/" namespace.
	assert.False(t, scopeCovers(scopes, "module", "read", "alice"))
	// A prefix that merely starts with the same letters is not the namespace.
	assert.False(t, scopeCovers(scopes, "module", "read", "alicia/one"))
}

func TestScopeCovers_MixedScopeList(t *testing.T) {
	scopes := []string{"module:read:alice/*", "module:push:alice/mymod"}

	assert.True(t, scopeCovers(scopes, "module", "read", "alice/other"))
	assert.True(t, scopeCovers(scopes, "module", "push", "alice/mymod"))
	assert.False(t, scopeCovers(scopes, "module", "push", "alice/other"))
	assert.False(t, scopeCovers(scopes, "module", "read", "bob/x"))
}

func TestScopeCovers_IgnoresMalformedEntries(t *testing.T) {
	// A malformed entry must never widen access.
	scopes := []string{"", ":", "module", "module:", ":read", "module:read:"}
	assert.False(t, scopeCovers(scopes, "module", "read", "alice/mymod"))
}

func TestDomainMatches(t *testing.T) {
	cases := []struct {
		pattern, domain string
		want            bool
	}{
		{"", "alice/mymod", true}, // no restriction
		{"", "", true},
		{"alice/mymod", "alice/mymod", true},
		{"alice/mymod", "alice/other", false},
		{"alice/*", "alice/anything", true},
		{"alice/*", "alice", false},
		{"alice/*", "bob/x", false},
		{"alice/*", "", false},
		{"alice/mymod", "", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, constants.DomainMatches(tc.pattern, tc.domain),
			"DomainMatches(%q, %q)", tc.pattern, tc.domain)
	}
}

func TestIsKnownScope_BothForms(t *testing.T) {
	assert.True(t, constants.IsKnownScope("module:read"))
	assert.True(t, constants.IsKnownScope("module:*"))
	assert.True(t, constants.IsKnownScope("module:read:alice/mymod"))
	assert.True(t, constants.IsKnownScope("module:*:alice/*"))
	assert.True(t, constants.IsKnownScope(constants.ScopeOn(constants.ResourceModule, constants.ActionPush, "alice/mymod")))

	assert.False(t, constants.IsKnownScope("module:teleport:alice/x"))
	assert.False(t, constants.IsKnownScope("module:read:"))
	assert.False(t, constants.IsKnownScope("module:read:alice"), "a domain needs owner/name")
	assert.False(t, constants.IsKnownScope("module:read:*/x"), "an owner wildcard is not allowed")
	assert.False(t, constants.IsKnownScope("module:read:alice/my*mod"), "partial name wildcards are not allowed")
	assert.False(t, constants.IsKnownScope("module:read:a/b/c"))
}
