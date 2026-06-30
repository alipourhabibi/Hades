package config

import "fmt"

// DatabaseBackend selects the metadata storage implementation.
type DatabaseBackend string

const (
	DatabaseSQLite   DatabaseBackend = "sqlite"
	DatabasePostgres DatabaseBackend = "postgres"
)

func (d DatabaseBackend) Validate() error {
	switch d {
	case DatabaseSQLite, DatabasePostgres, "":
		return nil
	default:
		return fmt.Errorf("backends.database: unknown value %q (valid: sqlite, postgres)", d)
	}
}

// CacheBackend selects the cache/rate-limit implementation.
type CacheBackend string

const (
	CacheMemory CacheBackend = "memory"
	CacheRedis  CacheBackend = "redis"
)

func (c CacheBackend) Validate() error {
	switch c {
	case CacheMemory, CacheRedis, "":
		return nil
	default:
		return fmt.Errorf("backends.cache: unknown value %q (valid: memory, redis)", c)
	}
}

// GitBackend selects the git versioning implementation.
type GitBackend string

const (
	GitGoGit  GitBackend = "gogit"
	GitGitaly GitBackend = "gitaly"
)

func (g GitBackend) Validate() error {
	switch g {
	case GitGoGit, GitGitaly, "":
		return nil
	default:
		return fmt.Errorf("backends.git: unknown value %q (valid: gogit, gitaly)", g)
	}
}

// ArtifactBackend selects the SDK artifact storage implementation.
type ArtifactBackend string

const (
	ArtifactDisk   ArtifactBackend = "disk"
	ArtifactGitaly ArtifactBackend = "gitaly"
	ArtifactS3     ArtifactBackend = "s3"
)

func (a ArtifactBackend) Validate() error {
	switch a {
	case ArtifactDisk, ArtifactGitaly, ArtifactS3, "":
		return nil
	default:
		return fmt.Errorf("backends.artifactStorage: unknown value %q (valid: disk, gitaly, s3)", a)
	}
}

// BackendsConfig selects which backend implementation to use for each subsystem.
// Unset fields default to the zero-dep local implementations.
type BackendsConfig struct {
	// Database selects the metadata storage backend: "sqlite" (default) or "postgres".
	Database DatabaseBackend `json:"database" yaml:"database"`
	// Cache selects the cache/rate-limit backend: "memory" (default) or "redis".
	Cache CacheBackend `json:"cache" yaml:"cache"`
	// Git selects the git versioning backend: "gogit" (default) or "gitaly".
	Git GitBackend `json:"git" yaml:"git"`
	// ArtifactStorage selects the SDK artifact storage backend: "disk" (default), "gitaly", or "s3".
	ArtifactStorage ArtifactBackend `json:"artifactStorage" yaml:"artifactStorage"`
}

func (b BackendsConfig) Validate() error {
	if err := b.Database.Validate(); err != nil {
		return err
	}
	if err := b.Cache.Validate(); err != nil {
		return err
	}
	if err := b.Git.Validate(); err != nil {
		return err
	}
	return b.ArtifactStorage.Validate()
}

// SQLiteConfig holds configuration for the embedded SQLite metadata backend.
type SQLiteConfig struct {
	// Path is the file-system path to the SQLite database file.
	// Defaults to "./hades.db" when empty.
	Path string `json:"path" yaml:"path"`
}

// GitStorageConfig holds configuration for the local go-git storage backend.
type GitStorageConfig struct {
	// Root is the directory under which bare git repositories are stored.
	// Defaults to "./data/repos" when empty.
	Root string `json:"root" yaml:"root"`
}

// DiskStorageConfig holds configuration for the local disk SDK artifact backend.
type DiskStorageConfig struct {
	// Path is the directory under which SDK artifacts are stored.
	// Defaults to "./data/artifacts" when empty.
	Path string `json:"path" yaml:"path"`
}
