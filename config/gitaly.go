package config

// Gitaly holds the connection parameters for the Gitaly gRPC server.
type Gitaly struct {
	Port               int    `json:"port" yaml:"port"`
	DefaultStorageName string `json:"defaultStorageName" yaml:"defaultStorageName"`
}
