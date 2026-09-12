package content_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	authorizationsvc "github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/server/content"
	"github.com/alipourhabibi/Hades/internal/hades/server/module"
	notificationdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
)

// End-to-end tests for the push path: a real database, a real git repository,
// a real OPA engine deciding permissions, and the production handlers wired
// together the way the server wires them.
//
// Nothing is stubbed. The permission answers come from the policy the server
// ships, the commits are real git commits, and the rows are read back through
// the same storage the API reads.

type stack struct {
	env    *testsupport.Env
	upload *content.Handler
	module *module.Server
	authz  *authorizationsvc.Server
}

// newStack wires the production handlers over real infrastructure.
//
// lintEnabled is off in most tests: the linter shells out to buf, and a suite
// that needs a binary installed is a suite that does not run. The one test that
// exercises lint skips itself when buf is absent.
func newStack(t *testing.T, lintEnabled bool) *stack {
	t.Helper()

	env := testsupport.NewEnv(t)
	ctx := context.Background()

	engine, err := authorizationengine.New(ctx, env.DB.OPABinding(), cache.NewMemoryCache(), time.Minute)
	require.NoError(t, err)

	authz := authorizationsvc.NewServer(env.Logger, env.DB.User(), env.DB.Session(), engine)

	deps := &server.Dependencies{
		Logger:         env.Logger,
		ModuleDB:       env.DB.Module(),
		CommitDB:       env.DB.Commit(),
		OrgDB:          env.DB.Org(),
		UserDB:         env.DB.User(),
		SDKJobDB:       env.DB.SDKJob(),
		CIRunDB:        env.DB.CIRun(),
		NotificationDB: env.DB.Notification(),
		GitStorage:     env.Git,
		Authorization:  authz,
		UoW:            env.DB,
		OPAEngine:      engine,
		SDKConfig: config.SDKConfig{
			Enabled:     true,
			LintEnabled: lintEnabled,
			BufBin:      "buf",
			Generators: []config.GeneratorConfig{
				{Language: "go", Plugin: "buf.build/protocolbuffers/go"},
			},
		},
	}
	if lintEnabled {
		deps.ProtoLinter = lint.New("buf")
	}

	return &stack{
		env:    env,
		upload: content.NewHandler(deps),
		module: module.NewServer(deps),
		authz:  authz,
	}
}

// user creates an account and grants it the namespace-wide owner binding that
// registration normally grants, so it can create and push to its own modules.
func (s *stack) user(t *testing.T, username string) (*identityv1.User, context.Context) {
	t.Helper()
	user := s.env.CreateUser(t, username, username+"@example.com")
	require.NoError(t, s.authz.AddBasicRoles(context.Background(), username))
	return user, context.WithValue(context.Background(), constants.ContextKeyUser, user)
}

func protoFile(pkg string) string {
	return "syntax = \"proto3\";\n\npackage " + pkg + ";\n\n// A is a message.\nmessage A {\n  // id identifies the thing.\n  string id = 1;\n}\n"
}

// TestPushCreatesCommitFilesAndJobs walks the whole push: module creation
// through the real handler, an upload through the real handler, then reads the
// effects back out of git and the database.
func TestPushCreatesCommitFilesAndJobs(t *testing.T) {
	s := newStack(t, false)
	alice, ctx := s.user(t, "alice")

	created, err := s.module.CreateModuleByName(ctx, connect.NewRequest(&registryv1.CreateModuleByNameRequest{
		Name:       "mymod",
		Visibility: registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
	}))
	require.NoError(t, err)
	mod := created.Msg.Module
	assert.Equal(t, "alice/mymod", mod.Name)

	commits, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}},
	}})
	require.NoError(t, err)
	require.Len(t, commits, 1)

	// The file is really in the repository, at the path it was pushed to.
	content, size, err := s.env.Git.GetFile(ctx, "alice/mymod", commits[0].CommitHash, "foo/v1/a.proto")
	require.NoError(t, err)
	assert.Equal(t, protoFile("foo.v1"), string(content))
	assert.Equal(t, int64(len(protoFile("foo.v1"))), size)

	// The commit row is readable through the API's own storage, joined up.
	stored, err := s.env.DB.Commit().GetByHash(ctx, commits[0].CommitHash)
	require.NoError(t, err)
	assert.Equal(t, "alice/mymod", stored.Module.Name)
	assert.Equal(t, alice.Id, stored.CreatedByUserId)

	// One SDK job per configured generator, enqueued in the same transaction.
	jobs, err := s.env.DB.SDKJob().ListByModule(ctx, mod.Id)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "go", jobs[0].Language)
	assert.Equal(t, "pending", jobs[0].Status)
	assert.Equal(t, stored.Id, jobs[0].CommitID)
}

// TestPushIsDeduplicatedByContentDigest covers the digest short-circuit: the
// same files pushed twice must not create a second commit.
func TestPushIsDeduplicatedByContentDigest(t *testing.T) {
	s := newStack(t, false)
	_, ctx := s.user(t, "alice")
	mod := s.mustCreateModule(t, ctx, "mymod")

	files := []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}}
	first, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"}, Files: files,
	}})
	require.NoError(t, err)

	second, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"}, Files: files,
	}})
	require.NoError(t, err)

	assert.Equal(t, first[0].Id, second[0].Id, "identical content resolves to the commit already stored")

	all, err := s.env.DB.Commit().ListByModule(ctx, mod.Id, 50, 0)
	require.NoError(t, err)
	assert.Len(t, all, 2, "the initial commit from module creation, plus one push")
}

// TestPushReplacesModuleContents covers the upload contract: the files in a
// push are the module, so a file the push omits is gone from the new commit.
//
// This replaced a test asserting the opposite, that a push merges into the
// previous commit's file set. The merge was the reason a proto file could never
// be removed from a module: deleted or renamed away, it stayed published and
// stayed readable by direct path, and there was no way to withdraw a schema
// short of deleting the module. It also made the stored digest describe
// something the client never sent, so a client verifying what it downloaded got
// a mismatch.
//
// buf push sends the complete module content, so full replacement is what the
// protocol means. The cost is real and worth stating: a caller that pushes a
// subset now truncates the module rather than adding to it.
func TestPushReplacesModuleContents(t *testing.T) {
	s := newStack(t, false)
	_, ctx := s.user(t, "alice")
	s.mustCreateModule(t, ctx, "mymod")

	_, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}},
	}})
	require.NoError(t, err)

	second, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "foo/v1/b.proto", Content: []byte(protoFile("foo.v1"))}},
	}})
	require.NoError(t, err)

	_, _, err = s.env.Git.GetFile(ctx, "alice/mymod", second[0].CommitHash, "foo/v1/b.proto")
	assert.NoError(t, err, "the pushed file must be present")

	_, _, err = s.env.Git.GetFile(ctx, "alice/mymod", second[0].CommitHash, "foo/v1/a.proto")
	assert.ErrorIs(t, err, gitstorage.ErrNotFound,
		"a file the push omitted must be absent from the new commit")
}

// TestPushToAnotherUsersModuleIsDenied is the authorisation boundary, decided
// by the real policy engine rather than a stub.
func TestPushToAnotherUsersModuleIsDenied(t *testing.T) {
	s := newStack(t, false)
	_, aliceCtx := s.user(t, "alice")
	s.mustCreateModule(t, aliceCtx, "mymod")

	_, bobCtx := s.user(t, "bob")

	_, err := s.upload.Upload(bobCtx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}},
	}})

	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

// TestPushNotifiesTheOrgMembers covers the notification producer added in 3.4,
// including the rule that an organisation's members are notified rather than
// the organisation row, and never the person who pushed.
func TestPushNotifiesTheOrgMembers(t *testing.T) {
	s := newStack(t, false)
	alice, aliceCtx := s.user(t, "alice")
	bob, _ := s.user(t, "bob")

	org := s.env.CreateOrg(t, "acme", alice)
	// CreateOrg here is a storage call; the OrgService handler pairs it with
	// these two bindings, so the test grants what the handler would.
	require.NoError(t, s.authz.AddOrgOwner(context.Background(), "alice", "acme"))
	require.NoError(t, s.env.DB.Org().AddMember(context.Background(), org.Id, bob.Id, "member"))
	require.NoError(t, s.authz.AddOrgMemberBinding(context.Background(), "bob", constants.RoleContributor, "acme"))

	created, err := s.module.CreateModuleByName(aliceCtx, connect.NewRequest(&registryv1.CreateModuleByNameRequest{
		Name: "shared", Owner: "acme", Visibility: registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
	}))
	require.NoError(t, err)
	assert.Equal(t, "acme/shared", created.Msg.Module.Name)

	_, err = s.upload.Upload(aliceCtx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "acme", Module: "shared"},
		Files:     []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}},
	}})
	require.NoError(t, err)

	bobNotifications, err := s.env.DB.Notification().ListForUser(context.Background(), bob.Id, 50, 0)
	require.NoError(t, err)
	require.Len(t, bobNotifications, 1, "a member of the owning org hears about the push")
	assert.Equal(t, notificationdb.TypeCommitPushed, bobNotifications[0].Type)
	assert.Contains(t, bobNotifications[0].Body, "alice pushed")

	aliceNotifications, err := s.env.DB.Notification().ListForUser(context.Background(), alice.Id, 50, 0)
	require.NoError(t, err)
	assert.Empty(t, aliceNotifications, "the person who pushed is not told about their own push")
}

// TestPushRejectsUnsafePaths covers the path validation that guards the
// temporary directory the lint and breaking checks write into.
func TestPushRejectsUnsafePaths(t *testing.T) {
	s := newStack(t, false)
	_, ctx := s.user(t, "alice")
	s.mustCreateModule(t, ctx, "mymod")

	for _, path := range []string{"../escape.proto", "/absolute.proto", "a/../../b.proto"} {
		t.Run(path, func(t *testing.T) {
			_, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
				ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
				Files:     []*registryv1.File{{Path: path, Content: []byte(protoFile("foo.v1"))}},
			}})
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestPushEnforcesTheFileCountLimit(t *testing.T) {
	s := newStack(t, false)
	_, ctx := s.user(t, "alice")
	s.mustCreateModule(t, ctx, "mymod")

	files := make([]*registryv1.File, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		files = append(files, &registryv1.File{Path: "foo/v1/" + name + ".proto", Content: []byte(protoFile("foo.v1"))})
	}
	s.upload = withUploadLimits(t, s, 2, 0)

	_, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"}, Files: files,
	}})

	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
}

// TestPushRunsLintAndRecordsTheResult is the one test that needs the buf
// binary, because the linter shells out to it. It covers both directions:
// a clean push records a CI run, and a lint violation rejects the push.
func TestPushRunsLintAndRecordsTheResult(t *testing.T) {
	if _, err := exec.LookPath("buf"); err != nil {
		t.Skip("buf binary not installed; lint and breaking checks cannot run")
	}

	s := newStack(t, true)
	_, ctx := s.user(t, "alice")
	mod := s.mustCreateModule(t, ctx, "mymod")

	commits, err := s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "foo/v1/a.proto", Content: []byte(protoFile("foo.v1"))}},
	}})
	require.NoError(t, err, "a file that satisfies the default preset must push cleanly")

	run, err := s.env.DB.CIRun().GetByModuleAndCommit(ctx, mod.Id, commits[0].CommitHash)
	require.NoError(t, err, "a push that ran the checks records the result")
	assert.True(t, run.LintPassed)
	// This module has breaking checks disabled and no predecessor to compare
	// against, so no comparison ran and the record must not claim one passed.
	assert.False(t, run.BreakingRan)
	assert.False(t, run.BreakingPassed, "breaking_passed is a claim about a comparison that happened")

	// A file whose package does not match its directory violates the default
	// preset, so the push is refused.
	before, err := s.env.DB.Commit().ListByModule(ctx, mod.Id, 50, 0)
	require.NoError(t, err)

	_, err = s.upload.Upload(ctx, []*registryv1.UploadRequestContent{{
		ModuleRef: &registryv1.ModuleRef{Owner: "alice", Module: "mymod"},
		Files:     []*registryv1.File{{Path: "wrong/place/b.proto", Content: []byte(protoFile("totally.unrelated"))}},
	}})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	after, err := s.env.DB.Commit().ListByModule(ctx, mod.Id, 50, 0)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "a rejected push leaves no commit behind")
}

// --- helpers ---------------------------------------------------------------

func (s *stack) mustCreateModule(t *testing.T, ctx context.Context, name string) *registryv1.Module {
	t.Helper()
	created, err := s.module.CreateModuleByName(ctx, connect.NewRequest(&registryv1.CreateModuleByNameRequest{
		Name:       name,
		Visibility: registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
	}))
	require.NoError(t, err)
	return created.Msg.Module
}

// withUploadLimits rebuilds the upload handler with tighter caps.
func withUploadLimits(t *testing.T, s *stack, maxFiles int, maxBytes int64) *content.Handler {
	t.Helper()
	return content.NewHandler(&server.Dependencies{
		Logger:         s.env.Logger,
		ModuleDB:       s.env.DB.Module(),
		CommitDB:       s.env.DB.Commit(),
		OrgDB:          s.env.DB.Org(),
		SDKJobDB:       s.env.DB.SDKJob(),
		CIRunDB:        s.env.DB.CIRun(),
		NotificationDB: s.env.DB.Notification(),
		GitStorage:     s.env.Git,
		Authorization:  s.authz,
		UoW:            s.env.DB,
		SDKConfig: config.SDKConfig{
			MaxUploadFiles: maxFiles,
			MaxUploadBytes: maxBytes,
		},
	})
}
