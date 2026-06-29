package git

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
)

// BackendType is the name of a git backend implementation.
type BackendType string

const (
	BackendGoGit  BackendType = "gogit"
	BackendGitaly BackendType = "gitaly"
)

// SelectedBackend returns the effective backend name, defaulting to "gogit".
func SelectedBackend(cfg config.BackendsConfig) BackendType {
	switch cfg.Git {
	case "gitaly":
		return BackendGitaly
	case "gogit", "":
		return BackendGoGit
	default:
		return BackendType(cfg.Git)
	}
}

// ValidateBackend returns an error if the configured git backend name is unknown.
func ValidateBackend(cfg config.BackendsConfig) error {
	b := cfg.Git
	if b == "" || b == "gogit" || b == "gitaly" {
		return nil
	}
	return fmt.Errorf("git: unknown backend %q (valid: gogit, gitaly)", b)
}
