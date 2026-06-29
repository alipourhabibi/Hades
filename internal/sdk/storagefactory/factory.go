// Package storagefactory constructs the SDK artifact storage backend from config.
// It lives in its own package to avoid import cycles between the storage parent
// package (which defines the Backend interface) and its sub-packages (which
// implement it).
package storagefactory

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/sdk/storage/disk"
	gitalysdk "github.com/alipourhabibi/Hades/internal/sdk/storage/gitaly"
	"github.com/alipourhabibi/Hades/internal/sdk/storage/s3"
)

// New constructs the Backend selected by cfg.Backends.ArtifactStorage.
// gitStore is required when backend is "gitaly"; may be nil otherwise.
func New(cfg config.Config, gitStore gitstorage.Storage) (sdkstorage.Backend, error) {
	backend := cfg.Backends.ArtifactStorage
	if backend == "" {
		// Backwards-compatibility: fall back to sdk.storage.type.
		backend = cfg.SDK.Storage.Type
	}
	if backend == "" {
		backend = "disk"
	}

	switch backend {
	case "disk":
		return disk.New(cfg.DiskStorage.Path), nil
	case "gitaly":
		if gitStore == nil {
			return nil, fmt.Errorf("artifact storage: gitaly backend requires a git.Storage instance")
		}
		return gitalysdk.New(gitStore), nil
	case "s3":
		return s3.New(cfg.SDK.Storage.S3)
	default:
		return nil, fmt.Errorf("artifact storage: unknown backend %q (valid: disk, gitaly, s3)", backend)
	}
}
