package meta

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
	"github.com/alipourhabibi/Hades/utils/log"
)

// stubAuthz supplies the read decision; everything else is real storage.
type stubAuthz struct {
	err   error
	calls int
}

func (s *stubAuthz) CheckReadAccess(_ context.Context, _ *identityv1.User, _ []*registryv1.Module) error {
	s.calls++
	return s.err
}

type fixture struct {
	env    *testsupport.Env
	h      *Handler
	authz  *stubAuthz
	alice  *identityv1.User
	module *registryv1.Module
	ctx    context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	env := testsupport.NewEnv(t)
	authz := &stubAuthz{}
	logger, err := log.NewWithConfig(config.Logger{Level: "error", Output: "stdout"})
	require.NoError(t, err)

	alice := env.CreateUser(t, "alice", "alice@example.com")
	module := env.CreateModule(t, "alice/mymod", alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	return &fixture{
		env:    env,
		authz:  authz,
		alice:  alice,
		module: module,
		h: &Handler{
			logger:          logger,
			ciRunStorage:    env.DB.CIRun(),
			sdkJobStorage:   env.DB.SDKJob(),
			moduleDBStorage: env.DB.Module(),
			authz:           authz,
		},
		ctx: context.WithValue(context.Background(), constants.ContextKeyUser, alice),
	}
}

// --- GetCIRun --------------------------------------------------------------

func TestGetCIRun_ReturnsTheRecordedRun(t *testing.T) {
	f := newFixture(t)
	hash := f.env.Commit(t, f.module, f.alice, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	_, err := f.env.DB.CIRun().Create(context.Background(), f.module.Id, hash, true, true, nil, nil)
	require.NoError(t, err)

	resp, err := f.h.GetCIRun(f.ctx, connect.NewRequest(&registryv1.GetCIRunRequest{
		Owner: "alice", ModuleName: "mymod", CommitHash: hash,
	}))

	require.NoError(t, err)
	assert.True(t, resp.Msg.CiRun.LintPassed)
	assert.True(t, resp.Msg.CiRun.BreakingPassed)
	assert.Equal(t, hash, resp.Msg.CiRun.CommitHash)
}

func TestGetCIRun_CommitWithNoRunIsNotFound(t *testing.T) {
	// The initial commit that CreateModuleByName seeds runs no checks, and a
	// deployment with linting off records nothing, so this is a normal answer
	// rather than an error condition.
	f := newFixture(t)
	hash := f.env.Commit(t, f.module, f.alice, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.GetCIRun(f.ctx, connect.NewRequest(&registryv1.GetCIRunRequest{
		Owner: "alice", ModuleName: "mymod", CommitHash: hash,
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetCIRun_UnknownModuleIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.GetCIRun(f.ctx, connect.NewRequest(&registryv1.GetCIRunRequest{
		Owner: "alice", ModuleName: "nosuch", CommitHash: "abc",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Zero(t, f.authz.calls, "a module that does not exist is rejected before any policy question")
}

// TestGetCIRun_RunOfAnotherModuleIsNotVisible covers the lookup being keyed by
// module as well as commit. Commit hashes are unique registry-wide, so a lookup
// keyed on the hash alone would serve one module's CI result through another.
func TestGetCIRun_RunOfAnotherModuleIsNotVisible(t *testing.T) {
	f := newFixture(t)
	other := f.env.CreateModule(t, "alice/other", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
	hash := f.env.Commit(t, other, f.alice, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	_, err := f.env.DB.CIRun().Create(context.Background(), other.Id, hash, true, true, nil, nil)
	require.NoError(t, err)

	_, err = f.h.GetCIRun(f.ctx, connect.NewRequest(&registryv1.GetCIRunRequest{
		Owner: "alice", ModuleName: "mymod", CommitHash: hash,
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetCIRun_DeniedReadIsSurfaced(t *testing.T) {
	f := newFixture(t)
	f.authz.err = connect.NewError(connect.CodeNotFound, stubError("not found"))

	_, err := f.h.GetCIRun(f.ctx, connect.NewRequest(&registryv1.GetCIRunRequest{
		Owner: "alice", ModuleName: "mymod", CommitHash: "abc",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Equal(t, 1, f.authz.calls)
}

// --- ListSDKs --------------------------------------------------------------

func TestListSDKs_ReturnsEveryJobForTheModule(t *testing.T) {
	f := newFixture(t)
	hash := f.env.Commit(t, f.module, f.alice, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	stored, err := f.env.DB.Commit().GetByHash(context.Background(), hash)
	require.NoError(t, err)

	require.NoError(t, f.env.DB.SDKJob().CreateBatch(context.Background(), stored.Id, f.module.Id,
		[]config.GeneratorConfig{
			{Language: "go", Plugin: "buf.build/protocolbuffers/go"},
			{Language: "python", Plugin: "buf.build/protocolbuffers/python"},
		}))

	resp, err := f.h.ListSDKs(f.ctx, connect.NewRequest(&registryv1.ListSDKsRequest{
		Owner: "alice", Module: "mymod",
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.SdkJobs, 2)
	for _, job := range resp.Msg.SdkJobs {
		assert.Equal(t, "pending", job.Status, "a freshly enqueued job has not run yet")
		assert.Empty(t, job.OutputLocation)
		assert.Nil(t, job.UpdateTime, "update_time is unset until the job reaches a terminal state")
	}
}

func TestListSDKs_ModuleWithNoJobsReturnsEmpty(t *testing.T) {
	f := newFixture(t)

	resp, err := f.h.ListSDKs(f.ctx, connect.NewRequest(&registryv1.ListSDKsRequest{
		Owner: "alice", Module: "mymod",
	}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.SdkJobs)
}

func TestListSDKs_UnknownModuleIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.ListSDKs(f.ctx, connect.NewRequest(&registryv1.ListSDKsRequest{
		Owner: "alice", Module: "nosuch",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Zero(t, f.authz.calls)
}

func TestListSDKs_DeniedReadIsSurfaced(t *testing.T) {
	f := newFixture(t)
	f.authz.err = connect.NewError(connect.CodeNotFound, stubError("not found"))

	_, err := f.h.ListSDKs(f.ctx, connect.NewRequest(&registryv1.ListSDKsRequest{
		Owner: "alice", Module: "mymod",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

type stubError string

func (e stubError) Error() string { return string(e) }
