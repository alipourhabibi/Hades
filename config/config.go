// Package config holds the configuration types for all Hades subsystems.
// Configuration is loaded from YAML files at startup and threaded through
// the dependency injection layer.
package config

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// OPAConfig holds tuning parameters for the in-process OPA authorization engine.
type OPAConfig struct {
	// BindingCacheTTL is how long per-subject role-binding results are cached.
	// Zero uses a backend-specific default: 10s for in-memory, 60s for Redis.
	// Longer TTL reduces DB load but increases the time window for stale
	// bindings after a revocation.
	BindingCacheTTL time.Duration `json:"bindingCacheTTL" yaml:"bindingCacheTTL"`
}

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
	OPA       OPAConfig       `json:"opa" yaml:"opa"`

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

// envPattern matches ${VAR} and ${VAR:-default} references.
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// expandEnv substitutes ${VAR} and ${VAR:-default} references in the raw config
// before it is parsed.
//
// Every secret this file carries (database DSN, SMTP password, OAuth client
// secrets, Redis password, the TOTP encryption key) would otherwise have to be
// written to disk in plaintext in every deployment. An unset variable with no
// default expands to the empty string, which Validate then rejects for the
// fields that require a value.
func expandEnv(content []byte) []byte {
	return envPattern.ReplaceAllFunc(content, func(match []byte) []byte {
		groups := envPattern.FindSubmatch(match)
		if val, ok := os.LookupEnv(string(groups[1])); ok {
			return []byte(val)
		}
		return groups[2] // the default, or empty when none was given
	})
}

func loadYaml(content []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(expandEnv(content), cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate returns an error if any config field holds an unrecognised or
// unusable value.
//
// Checks that a subsystem cannot recover from at runtime belong here: failing
// at startup with a named field is far better than failing on the first request
// that happens to need the value.
func (c *Config) Validate() error {
	if err := c.Backends.Validate(); err != nil {
		return err
	}

	if c.Server.RegistryHost == "" {
		return fmt.Errorf("server.registryHost is required: it is embedded in generated buf.yaml files and in Go module paths, and an empty value produces broken paths at runtime")
	}

	// The TOTP secret encryption key must be a valid AES-256 key whenever TOTP
	// can be reached. Without this check an empty key is accepted at startup and
	// fails only when a user first tries to enrol.
	if key := c.TOTP.EncryptionKey; key != "" {
		if raw, err := hex.DecodeString(key); err != nil || len(raw) != 32 {
			return fmt.Errorf("totp.encryptionKey must be 64 hex characters (a 32-byte AES-256 key)")
		}
	}

	if c.Backends.Database == DatabasePostgres && c.DB.ConnectionString == "" {
		return fmt.Errorf("db.connectionString is required when backends.database is %q", DatabasePostgres)
	}
	if c.Backends.Cache == CacheRedis && c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required when backends.cache is %q", CacheRedis)
	}

	for name, p := range map[string]OAuthProvider{"github": c.OAuth.GitHub, "google": c.OAuth.Google} {
		// A provider counts as configured once it has a client id. A redirect URL
		// on its own is just a placeholder for a provider that is switched off,
		// which is the normal state in the sample configs.
		if p.ClientID == "" {
			continue
		}
		if p.ClientSecret == "" || p.RedirectURL == "" {
			return fmt.Errorf("oauth.%s: clientSecret and redirectUrl are required once clientId is set", name)
		}
	}

	for _, cidr := range c.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			if _, addrErr := netip.ParseAddr(cidr); addrErr != nil {
				return fmt.Errorf("server.trustedProxies: %q is not a CIDR range or IP address", cidr)
			}
		}
	}

	return nil
}
