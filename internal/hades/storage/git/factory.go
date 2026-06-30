package git

import (
	"github.com/alipourhabibi/Hades/config"
)

// ValidateBackend returns an error if the configured git backend name is unknown.
func ValidateBackend(cfg config.BackendsConfig) error {
	return cfg.Git.Validate()
}

// SelectedBackend returns the effective git backend, defaulting to gogit.
func SelectedBackend(cfg config.BackendsConfig) config.GitBackend {
	if cfg.Git == "" {
		return config.GitGoGit
	}
	return cfg.Git
}
