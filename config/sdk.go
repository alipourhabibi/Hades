package config

// SDKConfig configures the SDK code generation pipeline.
type SDKConfig struct {
	Enabled         bool   `yaml:"enabled"`
	BufBin          string `yaml:"bufBin"`
	LintEnabled     bool   `yaml:"lintEnabled"`
	BreakingEnabled bool   `yaml:"breakingEnabled"`
	// MaxUploadFiles caps how many files a single push may contain.
	// Zero uses the built-in default; see internal/hades/server/content.
	MaxUploadFiles int `json:"maxUploadFiles" yaml:"maxUploadFiles"`
	// MaxUploadBytes caps the total content size of a single push in bytes.
	// Zero uses the built-in default.
	MaxUploadBytes int64             `json:"maxUploadBytes" yaml:"maxUploadBytes"`
	Generators     []GeneratorConfig `yaml:"generators"`
	Storage        SDKStorageConfig  `yaml:"storage"`
}

type GeneratorConfig struct {
	Language string `yaml:"language"`
	// Plugin is the name or path of a locally installed protoc plugin binary,
	// e.g. "protoc-gen-go" or "/usr/local/bin/protoc-gen-go".
	Plugin  string `yaml:"plugin"`
	Options string `yaml:"options"`
}

type SDKStorageConfig struct {
	Type   string          `yaml:"type"`
	S3     S3Config        `yaml:"s3"`
	Gitaly GitalySDKConfig `yaml:"gitaly"`
}

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"accessKeyId"`
	SecretAccessKey string `yaml:"secretAccessKey"`
	UseSSL          bool   `yaml:"useSSL"`
	Region          string `yaml:"region"`
}

type GitalySDKConfig struct {
	BranchPrefix string `yaml:"branchPrefix"`
}
