package bufmodules

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	modulev1 "buf.build/gen/go/bufbuild/registry/protocolbuffers/go/buf/registry/module/v1"
)

// Mounting buf's ModuleService exposes its whole method set, but this adapter
// implements only GetModules. The struct used to embed the bare handler
// interface, which is nil, so calling any of the others dereferenced a nil
// interface and panicked. ListModules is reachable without a credential, so
// that was an unauthenticated remote panic.
//
// These call the methods the adapter does not implement and require an
// Unimplemented status instead.
func TestUnimplementedProceduresReturnUnimplemented(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

	cases := map[string]func() error{
		"ListModules": func() error {
			_, err := s.ListModules(ctx, connect.NewRequest(&modulev1.ListModulesRequest{}))
			return err
		},
		"CreateModules": func() error {
			_, err := s.CreateModules(ctx, connect.NewRequest(&modulev1.CreateModulesRequest{}))
			return err
		},
		"UpdateModules": func() error {
			_, err := s.UpdateModules(ctx, connect.NewRequest(&modulev1.UpdateModulesRequest{}))
			return err
		},
		"DeleteModules": func() error {
			_, err := s.DeleteModules(ctx, connect.NewRequest(&modulev1.DeleteModulesRequest{}))
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			assert.NotPanics(t, func() { err = call() })
			assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
		})
	}
}
