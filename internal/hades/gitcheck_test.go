package hades

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gogit"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
	"github.com/alipourhabibi/Hades/utils/log"
)

// These use a real database and real repositories on disk, because the bug this
// check exists for was precisely a path that pointed somewhere real code could
// not find, which no fake would have modelled.

func checkLogger(t *testing.T) *log.LoggerWrapper {
	t.Helper()
	logger, err := log.NewWithConfig(config.Logger{Level: "error", Output: "stdout"})
	require.NoError(t, err)
	return logger
}

// TestVerifyGitStorage_WrongRootIsFatal reproduces the failure that motivated
// this check: the repository tree was moved from data/ to _data/ and the
// configured root was left behind, so every push failed with "internal server
// error" and the real cause was discarded at the RPC boundary.
func TestVerifyGitStorage_WrongRootIsFatal(t *testing.T) {
	env := testsupport.NewEnv(t)
	alice := env.CreateUser(t, "alice", "alice@example.com")
	env.CreateModule(t, "alice/mymod", alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	wrongRoot := gogit.New(filepath.Join(env.Dir, "somewhere-else"))

	err := verifyGitStorage(context.Background(), checkLogger(t), env.DB.Module(), wrongRoot)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "somewhere-else", "the message must name the root actually in use")
	assert.Contains(t, err.Error(), "alice/mymod", "and a module it expected to find there")
	assert.Contains(t, err.Error(), "gitStorage.root", "and where to fix it")
}

// TestVerifyGitStorage_CorrectRootPasses is the same registry with the root the
// repositories are really under.
func TestVerifyGitStorage_CorrectRootPasses(t *testing.T) {
	env := testsupport.NewEnv(t)
	alice := env.CreateUser(t, "alice", "alice@example.com")
	env.CreateModule(t, "alice/mymod", alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	assert.NoError(t, verifyGitStorage(context.Background(), checkLogger(t), env.DB.Module(), env.Git))
}

// TestVerifyGitStorage_FreshRegistryPasses covers a first run. There are no
// modules, the root has not been created yet, and neither is a problem.
func TestVerifyGitStorage_FreshRegistryPasses(t *testing.T) {
	env := testsupport.NewEnv(t)
	notYetCreated := gogit.New(filepath.Join(env.Dir, "will-exist-after-first-push"))

	assert.NoError(t, verifyGitStorage(context.Background(), checkLogger(t), env.DB.Module(), notYetCreated))
}

// TestVerifyGitStorage_OneBrokenModuleIsNotFatal is the distinction the check
// turns on. A single missing repository is damage to one module; refusing to
// boot would turn that into an outage for every other module in the registry.
func TestVerifyGitStorage_OneBrokenModuleIsNotFatal(t *testing.T) {
	env := testsupport.NewEnv(t)
	alice := env.CreateUser(t, "alice", "alice@example.com")
	env.CreateModule(t, "alice/healthy", alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	// A module row whose repository was never created, standing in for one that
	// was deleted or corrupted out from under the registry.
	_, err := env.DB.Module().Create(context.Background(), "alice/broken", alice.Id,
		registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC,
		registryv1.ModuleState_MODULE_STATE_ACTIVE, "", "", "", "main",
		registryv1.LintPreset_LINT_PRESET_DEFAULT, false)
	require.NoError(t, err)

	assert.NoError(t, verifyGitStorage(context.Background(), checkLogger(t), env.DB.Module(), env.Git),
		"one broken module must not stop the server")
}

// TestVerifyGitStorage_RemoteBackendIsSkipped covers a backend whose
// repositories are not on this machine, so there is no local root to check.
func TestVerifyGitStorage_RemoteBackendIsSkipped(t *testing.T) {
	env := testsupport.NewEnv(t)
	alice := env.CreateUser(t, "alice", "alice@example.com")
	env.CreateModule(t, "alice/mymod", alice, registryv1.ModuleVisibility_MODULE_VISIBILITY_PUBLIC)

	assert.NoError(t, verifyGitStorage(context.Background(), checkLogger(t), env.DB.Module(), remoteBackend{}))
}

// remoteBackend implements git.Storage without the local-root capability, the
// way the Gitaly backend does.
type remoteBackend struct{ gitstorage.Storage }
