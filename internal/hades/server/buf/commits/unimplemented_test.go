package bufcommits

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
)

// This adapter implements GetCommits but not ListCommits, which buf's
// CommitService also declares and which is reachable without a credential.
// Embedding the bare handler interface made that call panic on a nil
// interface; the Unimplemented embed answers instead.
func TestListCommitsReturnsUnimplemented(t *testing.T) {
	s := &Server{}

	var err error
	assert.NotPanics(t, func() {
		_, err = s.ListCommits(context.Background(), connect.NewRequest(&modulev1.ListCommitsRequest{}))
	})
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
