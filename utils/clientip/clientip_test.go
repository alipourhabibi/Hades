package clientip

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrustedProxies(t *testing.T) {
	tp, err := ParseTrustedProxies([]string{"10.0.0.0/8", "192.168.1.5", " 172.16.0.0/12 ", ""})
	require.NoError(t, err)
	assert.Len(t, tp, 3)

	_, err = ParseTrustedProxies([]string{"not-an-ip"})
	assert.Error(t, err)
}

func TestFromHeaders_NoTrustedProxiesIgnoresHeaders(t *testing.T) {
	// This is the default configuration. A client that sends X-Forwarded-For
	// must not be able to choose its own rate-limit key or audit-log IP.
	got := FromHeaders("203.0.113.9:44321", "1.2.3.4", "5.6.7.8", nil)
	assert.Equal(t, "203.0.113.9", got)
}

func TestFromHeaders_UntrustedPeerIgnoresHeaders(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("203.0.113.9:44321", "1.2.3.4", "", trusted)
	assert.Equal(t, "203.0.113.9", got, "a peer outside the trusted set must not be believed")
}

func TestFromHeaders_TrustedPeerUsesForwardedFor(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("10.1.2.3:9000", "203.0.113.7", "", trusted)
	assert.Equal(t, "203.0.113.7", got)
}

func TestFromHeaders_WalksChainFromTheRight(t *testing.T) {
	// The attacker controls the leftmost entries: they can prepend anything.
	// Only the rightmost non-trusted hop was actually observed by infrastructure
	// we trust.
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("10.1.2.3:9000", "1.1.1.1, 203.0.113.7, 10.0.0.5", "", trusted)
	assert.Equal(t, "203.0.113.7", got)
}

func TestFromHeaders_FallsBackToRealIP(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("10.1.2.3:9000", "", "203.0.113.7", trusted)
	assert.Equal(t, "203.0.113.7", got)
}

func TestFromHeaders_AllHopsTrustedFallsBackToPeer(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("10.1.2.3:9000", "10.0.0.5, 10.0.0.6", "", trusted)
	assert.Equal(t, "10.1.2.3", got)
}

func TestFromHeaders_IPv6Peer(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"::1/128"})
	require.NoError(t, err)

	got := FromHeaders("[::1]:9000", "203.0.113.7", "", trusted)
	assert.Equal(t, "203.0.113.7", got)
}

func TestFromHeaders_GarbageHopsAreSkipped(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	got := FromHeaders("10.1.2.3:9000", "not-an-ip, 203.0.113.7", "", trusted)
	assert.Equal(t, "203.0.113.7", got)
}
