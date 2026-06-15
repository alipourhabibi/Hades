package config

// Gitaly holds the connection parameters for the Gitaly gRPC server.
type Gitaly struct {
	Host               string `json:"host" yaml:"host"`
	Port               int    `json:"port" yaml:"port"`
	DefaultStorageName string `json:"defaultStorageName" yaml:"defaultStorageName"`
}
