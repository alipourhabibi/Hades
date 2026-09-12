package config_test

import (
	"testing"

	"github.com/alipourhabibi/Hades/config"
)

// TestContainerYAMLStartsWithNoOperatorInput pins the claim CLAUDE.md makes
// about the image: container.yaml "starts with no operator input and nothing
// else running".
//
// It did not. registryHost read "${HADES_REGISTRY_HOST}" with no default, an
// unset variable expands to the empty string, and Validate rejects that, so
// `docker run ghcr.io/alipourhabibi/hades` exited immediately on a config
// error. Both spellings are covered here, unset and set-but-empty, because the
// expansion used to treat them differently and only the first was ever tried.
func TestContainerYAMLStartsWithNoOperatorInput(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		if _, err := config.LoadFile("container.yaml"); err != nil {
			t.Fatalf("container.yaml must load with no environment: %v", err)
		}
	})

	t.Run("set but empty", func(t *testing.T) {
		t.Setenv("HADES_REGISTRY_HOST", "")
		t.Setenv("HADES_TOTP_ENCRYPTION_KEY", "")
		if _, err := config.LoadFile("container.yaml"); err != nil {
			t.Fatalf("an empty variable must fall back to the default: %v", err)
		}
	})

	t.Run("set", func(t *testing.T) {
		t.Setenv("HADES_REGISTRY_HOST", "registry.example.com")
		cfg, err := config.LoadFile("container.yaml")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Server.RegistryHost != "registry.example.com" {
			t.Fatalf("registryHost = %q, want the environment value", cfg.Server.RegistryHost)
		}
	})
}
