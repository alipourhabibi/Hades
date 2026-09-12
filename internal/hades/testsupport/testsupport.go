// Package testsupport builds real Hades infrastructure for tests: a migrated
// SQLite database and a go-git repository root, both on a temporary directory
// that the test framework removes afterwards.
//
// It exists because this project does not mock its storage. A fake that returns
// canned rows cannot catch a wrong JOIN, a missing scope on an UPDATE, a
// constraint the code violates, or a SQLite/PostgreSQL divergence, which is
// where the bugs in this codebase have actually been. Everything here runs the
// production constructors, so a test failure means production is broken rather
// than a fake drifted.
//
// The package is not named with a _test suffix because several test packages
// import it. Nothing in the server links it.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/hades/storage/git/gogit"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

// Env is a working Hades storage layer: a migrated database and a git backend.
type Env struct {
	// DB is a fully migrated store, built by the same constructor the server
	// uses, so the schema under test is the schema that ships.
	DB db.Store
	// Git is a real go-git backend writing to a temporary directory. Commits
	// made through it are real commits, readable with any git tool.
	Git gitstorage.Storage
	// Dir is the temporary root holding both.
	Dir string
	// Logger is wired to stdout at error level so a passing test stays quiet.
	Logger *log.LoggerWrapper
}

// NewEnv builds an Env on a temporary directory.
//
// The SQLite database is a file rather than :memory: on purpose: the shared
// in-memory DSN is process-global, so two tests running in the same package
// would share one database and see each other's rows.
func NewEnv(t *testing.T) *Env {
	t.Helper()

	initTelemetry(t)

	dir := t.TempDir()
	logger, err := log.NewWithConfig(config.Logger{Level: "error", Output: "stdout"})
	if err != nil {
		t.Fatalf("testsupport: logger: %v", err)
	}

	cfg := config.Config{
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "hades.db")},
	}
	store, err := db.NewFromConfig(cfg, logger)
	if err != nil {
		t.Fatalf("testsupport: open database: %v", err)
	}

	repoRoot := filepath.Join(dir, "repos")
	if err := os.MkdirAll(repoRoot, 0o750); err != nil {
		t.Fatalf("testsupport: repo root: %v", err)
	}

	return &Env{
		DB:     store,
		Git:    gogit.New(repoRoot),
		Dir:    dir,
		Logger: logger,
	}
}

// CreateUser inserts a user account and returns the stored record.
func (e *Env) CreateUser(t *testing.T, username, email string) *identityv1.User {
	t.Helper()
	ctx := context.Background()

	if err := e.DB.User().Create(ctx, username, email, "$2a$10$notarealhashbutlongenough",
		identityv1.UserType_USER_TYPE_USER, identityv1.UserState_USER_STATE_ACTIVE, "", ""); err != nil {
		t.Fatalf("testsupport: create user %q: %v", username, err)
	}
	user, err := e.DB.User().GetByUsername(ctx, username)
	if err != nil {
		t.Fatalf("testsupport: read back user %q: %v", username, err)
	}
	return user
}

// CreateOrg inserts an organisation owned by creator and returns it.
func (e *Env) CreateOrg(t *testing.T, name string, creator *identityv1.User) *identityv1.User {
	t.Helper()

	org, err := e.DB.Org().Create(context.Background(), name, "", "", creator.Id)
	if err != nil {
		t.Fatalf("testsupport: create org %q: %v", name, err)
	}
	return org
}

// CreateModule inserts a module owned by owner and creates its git repository,
// in that order, so the module is immediately pushable.
func (e *Env) CreateModule(t *testing.T, fullName string, owner *identityv1.User, visibility registryv1.ModuleVisibility) *registryv1.Module {
	t.Helper()
	ctx := context.Background()

	module, err := e.DB.Module().Create(ctx, fullName, owner.Id, visibility,
		registryv1.ModuleState_MODULE_STATE_ACTIVE, "", "", "", "main",
		registryv1.LintPreset_LINT_PRESET_DEFAULT, false)
	if err != nil {
		t.Fatalf("testsupport: create module %q: %v", fullName, err)
	}
	if err := e.Git.CreateRepository(ctx, fullName, "main"); err != nil {
		t.Fatalf("testsupport: create repository %q: %v", fullName, err)
	}
	return module
}

// Commit writes files to a module's repository and records the commit row,
// returning the git commit hash. It mirrors what the upload handler does,
// without the lint and SDK machinery.
func (e *Env) Commit(t *testing.T, module *registryv1.Module, author *identityv1.User, message string, files map[string]string) string {
	t.Helper()
	ctx := context.Background()

	gitFiles := make([]*gitstorage.File, 0, len(files))
	for path, content := range files {
		gitFiles = append(gitFiles, &gitstorage.File{Path: path, Content: []byte(content)})
	}

	// ExpectedHead is read from the branch rather than passed in: this helper
	// exists to seed fixtures, not to exercise the compare-and-swap.
	var expectedHead string
	if commits, err := e.Git.ListCommits(ctx, module.Name, module.DefaultBranch, 1); err == nil && len(commits) > 0 {
		expectedHead = commits[0].SHA
	}
	existing, _ := e.Git.ListFiles(ctx, module.Name, module.DefaultBranch)

	hash, err := e.Git.PutFiles(ctx, gitstorage.PutFilesRequest{
		RepoPath:      module.Name,
		Branch:        module.DefaultBranch,
		Files:         gitFiles,
		ExistingPaths: existing,
		AuthorName:    author.Username,
		AuthorEmail:   author.Email,
		Message:       message,
		ExpectedHead:  expectedHead,
	})
	if err != nil {
		t.Fatalf("testsupport: put files in %q: %v", module.Name, err)
	}

	// Commit ids are derived from the git hash the same way the upload handler
	// derives them, so a test can look a commit up by either.
	id, err := commitUUID(hash)
	if err != nil {
		t.Fatalf("testsupport: commit id from hash %q: %v", hash, err)
	}
	if err := e.DB.Commit().Create(ctx, id, hash, module.OwnerId, module.Id,
		registryv1.DigestType_DIGEST_TYPE_B5, hash[:16], author.Id, ""); err != nil {
		t.Fatalf("testsupport: create commit row: %v", err)
	}
	return hash
}

// telemetryOnce guards the global OTel setup, which mutates package-level
// state and must run at most once per process.
var telemetryOnce sync.Once

// initTelemetry wires the no-op metric providers the server wires when
// telemetry is switched off.
//
// It is not optional. The package-level counters in internal/telemetry are nil
// interfaces until InitMetrics runs, and the upload path calls Add on one of
// them, so a test that skips this panics where production would not.
func initTelemetry(t *testing.T) {
	t.Helper()
	telemetryOnce.Do(func() {
		if _, err := telemetry.Setup(context.Background(), config.TelemetryConfig{}); err != nil {
			t.Fatalf("testsupport: telemetry setup: %v", err)
		}
	})
}

// commitUUID derives a commit row id from a git hash, the way the upload
// handler does: the first 32 hex characters of the hash parsed as a UUID.
func commitUUID(hash string) (uuid.UUID, error) {
	if len(hash) < 32 {
		return uuid.Nil, fmt.Errorf("testsupport: commit hash %q is too short", hash)
	}
	return uuid.Parse(hash[:32])
}
