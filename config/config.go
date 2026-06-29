// Package config holds the configuration types for all Hades subsystems.
// Configuration is loaded from YAML files at startup and threaded through
// the dependency injection layer.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration, aggregating all subsystem configs.
type Config struct {
	Logger    Logger          `json:"logger" yaml:"logger"`
	DB        DB              `json:"db" yaml:"db"`
	Server    Server          `json:"server" yaml:"server"`
	Gitaly    Gitaly          `json:"gitaly" yaml:"gitaly"`
	SDK       SDKConfig       `json:"sdk" yaml:"sdk"`
	Telemetry TelemetryConfig `json:"telemetry" yaml:"telemetry"`
	Auth      AuthConfig      `json:"auth" yaml:"auth"`
	Email     EmailConfig     `json:"email" yaml:"email"`
	OAuth     OAuthConfig     `json:"oauth" yaml:"oauth"`
	Redis     RedisConfig     `json:"redis" yaml:"redis"`
	TOTP      TOTPConfig      `json:"totp" yaml:"totp"`

	// Pluggable backend selectors and their per-backend configs.
	Backends    BackendsConfig    `json:"backends" yaml:"backends"`
	SQLite      SQLiteConfig      `json:"sqlite" yaml:"sqlite"`
	GitStorage  GitStorageConfig  `json:"gitStorage" yaml:"gitStorage"`
	DiskStorage DiskStorageConfig `json:"diskStorage" yaml:"diskStorage"`
}

// LoadFile reads and parses a YAML config file from the given path.
func LoadFile(filename string) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return loadYaml(content)
}

func loadYaml(content []byte) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal(content, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
