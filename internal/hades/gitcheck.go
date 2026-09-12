package hades

import (
	"context"
	"fmt"
	"strings"

	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/utils/log"
)

// localRepoStorage is the subset of behaviour a git backend needs for the
// startup check: repositories on this machine, at a knowable path, whose
// presence can be tested cheaply.
//
// Backends that do not implement it are skipped. Gitaly is the case in point:
// its repositories live on another host, so there is no local root to have
// pointed at the wrong place.
type localRepoStorage interface {
	RepoRoot() string
	RepositoryExists(ctx context.Context, repoPath string) (bool, error)
}

// gitRootProbeLimit bounds how many modules the check opens. The failure it
// looks for is a whole storage root that moved, which the first repository
// already reveals; reading more only sharpens the message.
const gitRootProbeLimit = 10

// verifyGitStorage refuses to start when the configured git storage root does
// not hold the repositories the database says exist.
//
// This exists because that mismatch is silent and unbounded: the server starts,
// every read and every push then fails deep inside a handler, and the caller is
// told "internal server error" with the real cause discarded at the boundary.
// A moved or mistyped root is worth one line at startup rather than one error
// per request forever after.
//
// It distinguishes a misconfigured root from an individual broken repository.
// Every probed module missing means the root is wrong, which is fatal. Some
// missing means those modules are damaged, which is a warning: the rest of the
// registry still works, and refusing to boot would turn one bad module into a
// full outage.
func verifyGitStorage(ctx context.Context, logger *log.LoggerWrapper, modules moduledb.Storage, git gitstorage.Storage) error {
	local, ok := git.(localRepoStorage)
	if !ok {
		return nil // remote backend; nothing local to verify
	}
	root := local.RepoRoot()

	sample, err := modules.ListModules(ctx, "", gitRootProbeLimit, 0)
	if err != nil {
		// Not fatal. The database is checked by its own startup path, and
		// failing here would turn a transient query error into a refusal to
		// boot over a diagnostic.
		logger.Warn("could not list modules to verify git storage", "error", err, "git_root", root)
		return nil
	}
	if len(sample) == 0 {
		// A registry with no modules yet. The root is created on first push, so
		// its absence now means nothing.
		logger.Info("git storage root", "path", root, "modules", 0)
		return nil
	}

	var missing []string
	for _, m := range sample {
		exists, err := local.RepositoryExists(ctx, m.Name)
		if err != nil {
			logger.Warn("could not check module repository", "error", err, "module", m.Name, "git_root", root)
			continue
		}
		if !exists {
			missing = append(missing, m.Name)
		}
	}

	switch {
	case len(missing) == len(sample):
		return fmt.Errorf(
			"git storage root %q holds none of the %d module repositories the database expects (for example %q): "+
				"the configured root does not match where the repositories actually are. "+
				"Check backends.git and gitStorage.root in the config",
			root, len(sample), missing[0])
	case len(missing) > 0:
		logger.Warn("some module repositories are missing from git storage",
			"git_root", root, "missing", strings.Join(missing, ", "),
			"missing_count", len(missing), "checked", len(sample))
	default:
		logger.Info("git storage root", "path", root, "modules_checked", len(sample))
	}
	return nil
}
