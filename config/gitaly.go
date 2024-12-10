package config

type Gitaly struct {
	Port               int    `json:"port" yaml:"port"`
	DefaultStorageName string `json:"defaultStorageName" yaml:"defaultStorageName"`
}
