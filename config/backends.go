package config

// BackendsConfig selects which backend implementation to use for each subsystem.
// Unset fields default to the zero-dep local implementations.
type BackendsConfig struct {
	// Metadata selects the metadata storage backend: "sqlite" (default) or "postgres".
	Metadata string `json:"metadata" yaml:"metadata"`
	// Cache selects the cache/rate-limit backend: "memory" (default) or "redis".
	Cache string `json:"cache" yaml:"cache"`
	// Git selects the git versioning backend: "gogit" (default) or "gitaly".
	Git string `json:"git" yaml:"git"`
	// ArtifactStorage selects the SDK artifact storage backend: "disk" (default), "gitaly", or "s3".
	ArtifactStorage string `json:"artifactStorage" yaml:"artifactStorage"`
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
