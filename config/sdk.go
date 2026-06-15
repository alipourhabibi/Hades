package config

// SDKConfig configures the SDK code generation pipeline.
type SDKConfig struct {
	Enabled         bool              `yaml:"enabled"`
	BufBin          string            `yaml:"bufBin"`
	ProtocBin       string            `yaml:"protocBin"`
	LintEnabled     bool              `yaml:"lintEnabled"`
	BreakingEnabled bool              `yaml:"breakingEnabled"`
	Generators      []GeneratorConfig `yaml:"generators"`
	Storage         SDKStorageConfig  `yaml:"storage"`
}

type GeneratorConfig struct {
	Language string `yaml:"language"`
	Plugin   string `yaml:"plugin"`
	Options  string `yaml:"options"`
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
