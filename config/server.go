package config

// Server holds the HTTP/gRPC listener configuration.
type Server struct {
	ListenPort   int    `json:"listenPort" yaml:"listenPort"`
	RegistryHost string `json:"registryHost" yaml:"registryHost"`
	CertFile     string `json:"certFile" yaml:"certFile"`
	CertKey      string `json:"certKey" yaml:"certKey"`
}
