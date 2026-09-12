package auth_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	authv1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
)

// A username becomes the first path segment of every repository its modules
// get and of every URL it appears at. RegisterRequest.username bounds only the
// length, so the character set is the server's to enforce.
//
// The check is the first thing Register does, before the rate limiter and
// before any storage call, which is what lets this run on a fixture that wires
// neither.
func TestRegister_RejectsAUsernameThatIsNotASafePathSegment(t *testing.T) {
	f := newTokenFixture(t)

	for _, name := range []string{
		"a/b",       // a second path segment
		"..",        // the parent directory
		".git",      // a name git itself treats specially
		"has space", // not addressable without escaping
		"-leading",  // punctuation at an edge
		"trailing.",
		"go", // reserved: shadows a real route
	} {
		_, err := f.srv.Register(context.Background(), connect.NewRequest(&authv1.RegisterRequest{
			Username: name,
			Password: "Corr3ctHorseBattery!",
			Email:    "someone@example.com",
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "username %q", name)
	}
}
