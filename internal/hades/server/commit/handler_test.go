package commit

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

// These run against a real migrated database and a real git repository, so the
// joins, the pagination arithmetic and the tree reads under test are the ones
// that ship. Only the authorisation decision is substituted, because what most
// of these tests are about is what the handler does with an allow or a deny.

// stubAuthz answers every read check the same way. CheckReadAccess is the only
// method the handler uses.
type stubAuthz struct {
	err error
	// calls records the module names each call was asked about, so a test can
	// assert that the check happened at all, and on the right module.
	calls [][]string
}

func (s *stubAuthz) CheckReadAccess(_ context.Context, _ *identityv1.User, modules []*registryv1.Module) error {
	names := make([]string, 0, len(modules))
	for _, m := range modules {
		names = append(names, m.Name)
	}
	s.calls = append(s.calls, names)
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
		env:   env,
		authz: authz,
		alice: alice,
		h: &Handler{
			logger:          logger,
			commitDBStorage: env.DB.Commit(),
			moduleDBStorage: env.DB.Module(),
			gitStorage:      env.Git,
			authz:           authz,
		},
		module: module,
		ctx:    context.WithValue(context.Background(), constants.ContextKeyUser, alice),
	}
}

func (f *fixture) commit(t *testing.T, message string, files map[string]string) string {
	t.Helper()
	return f.env.Commit(t, f.module, f.alice, message, files)
}

// --- ListCommits -----------------------------------------------------------

func TestListCommits_ReturnsCommitsForTheModule(t *testing.T) {
	f := newFixture(t)
	first := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	second := f.commit(t, "second", map[string]string{"a.proto": "syntax = \"proto3\";\n// two"})

	resp, err := f.h.ListCommits(f.ctx, connect.NewRequest(&registryv1.ListCommitsRequest{
		Owner: "alice", Module: "mymod",
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Commits, 2)
	assert.NotEqual(t, first, second)
	assert.Len(t, f.authz.calls, 1, "read access is checked once for the module, not once per commit")
}

func TestListCommits_UnknownModuleIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.ListCommits(f.ctx, connect.NewRequest(&registryv1.ListCommitsRequest{
		Owner: "alice", Module: "nosuch",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Empty(t, f.authz.calls, "a module that does not exist is rejected before any policy question")
}

func TestListCommits_DeniedReadIsSurfaced(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	f.authz.err = connect.NewError(connect.CodeNotFound, assertError("not found"))

	_, err := f.h.ListCommits(f.ctx, connect.NewRequest(&registryv1.ListCommitsRequest{
		Owner: "alice", Module: "mymod",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestListCommits_PaginationWalksTheWholeSet(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{"one", "two", "three"} {
		f.commit(t, body, map[string]string{"a.proto": "syntax = \"proto3\";\n// " + body})
	}

	first, err := f.h.ListCommits(f.ctx, connect.NewRequest(&registryv1.ListCommitsRequest{
		Owner: "alice", Module: "mymod", PageSize: 2,
	}))
	require.NoError(t, err)
	require.Len(t, first.Msg.Commits, 2)
	require.NotEmpty(t, first.Msg.NextPageToken, "a full page must offer a cursor")

	second, err := f.h.ListCommits(f.ctx, connect.NewRequest(&registryv1.ListCommitsRequest{
		Owner: "alice", Module: "mymod", PageSize: 2, PageToken: first.Msg.NextPageToken,
	}))
	require.NoError(t, err)
	require.Len(t, second.Msg.Commits, 1)
	assert.Empty(t, second.Msg.NextPageToken, "a short page is the last page")

	seen := map[string]bool{}
	for _, c := range append(first.Msg.Commits, second.Msg.Commits...) {
		assert.False(t, seen[c.Id], "commit %s returned on two pages", c.Id)
		seen[c.Id] = true
	}
	assert.Len(t, seen, 3)
}

// --- GetCommit -------------------------------------------------------------

func TestGetCommit_ByHash(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	resp, err := f.h.GetCommit(f.ctx, connect.NewRequest(&registryv1.GetCommitRequest{CommitHash: hash}))

	require.NoError(t, err)
	assert.Equal(t, hash, resp.Msg.Commit.CommitHash)
	assert.Equal(t, "alice/mymod", resp.Msg.Commit.Module.Name)
}

func TestGetCommit_UnknownHashIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.GetCommit(f.ctx, connect.NewRequest(&registryv1.GetCommitRequest{
		CommitHash: "0000000000000000000000000000000000000000",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// --- GetCommitDiff ---------------------------------------------------------

func TestGetCommitDiff_InitialCommitIsAllAdditions(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";\nmessage A {}\n"})

	resp, err := f.h.GetCommitDiff(f.ctx, connect.NewRequest(&registryv1.GetCommitDiffRequest{CommitHash: hash}))

	require.NoError(t, err)
	require.NotEmpty(t, resp.Msg.Diffs, "the first commit is diffed against an empty tree")
	assert.Greater(t, resp.Msg.TotalAdditions, int32(0))
	assert.Equal(t, int32(0), resp.Msg.TotalDeletions)
	for _, d := range resp.Msg.Diffs {
		assert.True(t, d.IsNewFile, "every file in the initial commit is new")
	}
}

func TestGetCommitDiff_SecondCommitDiffsAgainstItsParent(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";\nmessage A {}\n"})
	second := f.commit(t, "second", map[string]string{"a.proto": "syntax = \"proto3\";\nmessage A {}\nmessage B {}\n"})

	resp, err := f.h.GetCommitDiff(f.ctx, connect.NewRequest(&registryv1.GetCommitDiffRequest{CommitHash: second}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Diffs, 1, "only the changed file appears")
	assert.False(t, resp.Msg.Diffs[0].IsNewFile)
	assert.Equal(t, int32(1), resp.Msg.TotalAdditions)
}

// --- ListModuleFiles -------------------------------------------------------

func TestListModuleFiles_ReadsHeadWhenNoCommitNamed(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";", "sub/b.proto": "syntax = \"proto3\";"})

	resp, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod",
	}))

	require.NoError(t, err)
	names := entryNames(resp.Msg.Entries)
	assert.Contains(t, names, "a.proto")
	assert.Contains(t, names, "sub")
}

func TestListModuleFiles_ReadsTheNamedCommit(t *testing.T) {
	f := newFixture(t)
	first := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	f.commit(t, "second", map[string]string{"a.proto": "syntax = \"proto3\";", "b.proto": "syntax = \"proto3\";"})

	resp, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod", CommitHash: first,
	}))

	require.NoError(t, err)
	names := entryNames(resp.Msg.Entries)
	assert.Contains(t, names, "a.proto")
	assert.NotContains(t, names, "b.proto", "the older commit must not show a file added later")
}

// TestListModuleFiles_CommitFromAnotherModuleIsRefused covers the cross-module
// read: commit hashes are unique across the whole registry, so without the
// ownership check in resolveRef a caller authorised for one module could read
// any commit of any other module by passing its hash.
func TestListModuleFiles_CommitFromAnotherModuleIsRefused(t *testing.T) {
	f := newFixture(t)
	other := f.env.CreateModule(t, "alice/secret", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
	foreign := f.env.Commit(t, other, f.alice, "secret", map[string]string{"secret.proto": "syntax = \"proto3\";"})
	f.commit(t, "mine", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod", CommitHash: foreign,
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestListModuleFiles_UnknownCommitIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod", CommitHash: "0000000000000000000000000000000000000000",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestListModuleFiles_UnknownPathIsNotFound pins the mapping fixed in 3.1: a
// path that is not in the tree is a caller error, and reporting it as Internal
// made every typo look like an outage.
func TestListModuleFiles_UnknownPathIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod", Path: "nosuchdir",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestListModuleFiles_DeniedReadHappensBeforeAnyGitAccess(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	f.authz.err = connect.NewError(connect.CodeNotFound, assertError("not found"))

	_, err := f.h.ListModuleFiles(f.ctx, connect.NewRequest(&registryv1.ListModuleFilesRequest{
		Owner: "alice", Module: "mymod",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Len(t, f.authz.calls, 1)
}

// --- GetFileContent --------------------------------------------------------

func TestGetFileContent_ReturnsTheStoredBytes(t *testing.T) {
	f := newFixture(t)
	body := "syntax = \"proto3\";\nmessage A {}\n"
	f.commit(t, "first", map[string]string{"a.proto": body})

	resp, err := f.h.GetFileContent(f.ctx, connect.NewRequest(&registryv1.GetFileContentRequest{
		Owner: "alice", Module: "mymod", Path: "a.proto",
	}))

	require.NoError(t, err)
	assert.Equal(t, body, string(resp.Msg.Content))
	assert.Equal(t, int64(len(body)), resp.Msg.Size)
}

func TestGetFileContent_ReadsTheNamedCommit(t *testing.T) {
	f := newFixture(t)
	first := f.commit(t, "first", map[string]string{"a.proto": "// v1\n"})
	f.commit(t, "second", map[string]string{"a.proto": "// v2\n"})

	resp, err := f.h.GetFileContent(f.ctx, connect.NewRequest(&registryv1.GetFileContentRequest{
		Owner: "alice", Module: "mymod", Path: "a.proto", CommitHash: first,
	}))

	require.NoError(t, err)
	assert.Equal(t, "// v1\n", string(resp.Msg.Content))
}

func TestGetFileContent_MissingFileIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.GetFileContent(f.ctx, connect.NewRequest(&registryv1.GetFileContentRequest{
		Owner: "alice", Module: "mymod", Path: "nosuch.proto",
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetFileContent_CommitFromAnotherModuleIsRefused(t *testing.T) {
	f := newFixture(t)
	other := f.env.CreateModule(t, "alice/secret", f.alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PRIVATE)
	foreign := f.env.Commit(t, other, f.alice, "secret", map[string]string{"secret.proto": "syntax = \"proto3\";"})
	f.commit(t, "mine", map[string]string{"a.proto": "syntax = \"proto3\";"})

	_, err := f.h.GetFileContent(f.ctx, connect.NewRequest(&registryv1.GetFileContentRequest{
		Owner: "alice", Module: "mymod", Path: "secret.proto", CommitHash: foreign,
	}))

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// --- helpers used by the buf.build adapters --------------------------------

func TestGetCommits_ChecksReadAccessPerCommit(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	stored, err := f.env.DB.Commit().GetByHash(context.Background(), hash)
	require.NoError(t, err)

	commits, err := f.h.GetCommits(f.ctx, []string{stored.Id}, nil)

	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, hash, commits[0].CommitHash)
	assert.Equal(t, [][]string{{"alice/mymod"}}, f.authz.calls)
}

func TestGetCommits_DeniedCommitReturnsNothing(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	stored, err := f.env.DB.Commit().GetByHash(context.Background(), hash)
	require.NoError(t, err)
	f.authz.err = connect.NewError(connect.CodeNotFound, assertError("not found"))

	commits, err := f.h.GetCommits(f.ctx, []string{stored.Id}, nil)

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, commits, "a denied read must not return the commit alongside the error")
}

func TestGetGraph_ChecksReadAccessPerCommit(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	stored, err := f.env.DB.Commit().GetByHash(context.Background(), hash)
	require.NoError(t, err)

	commits, err := f.h.GetGraph(f.ctx, []string{stored.Id}, nil)

	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, [][]string{{"alice/mymod"}}, f.authz.calls)
}

func TestGetGraph_DeniedCommitReturnsNothing(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})
	stored, err := f.env.DB.Commit().GetByHash(context.Background(), hash)
	require.NoError(t, err)
	f.authz.err = connect.NewError(connect.CodeNotFound, assertError("not found"))

	commits, err := f.h.GetGraph(f.ctx, []string{stored.Id}, nil)

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, commits)
}

// --- resolveRef ------------------------------------------------------------

func TestResolveRef_EmptyHashMeansHead(t *testing.T) {
	f := newFixture(t)

	ref, err := f.h.resolveRef(f.ctx, f.module, "")

	require.NoError(t, err)
	assert.Equal(t, "HEAD", ref)
}

func TestResolveRef_KnownHashResolvesToItself(t *testing.T) {
	f := newFixture(t)
	hash := f.commit(t, "first", map[string]string{"a.proto": "syntax = \"proto3\";"})

	ref, err := f.h.resolveRef(f.ctx, f.module, hash)

	require.NoError(t, err)
	assert.Equal(t, hash, ref)
}

func entryNames(entries []*registryv1.FileEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

// assertError is a tiny error value for stubbing a denial.
type assertError string

func (e assertError) Error() string { return string(e) }
