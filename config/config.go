package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Logger Logger `json:"logger" yaml:"logger"`
}

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
