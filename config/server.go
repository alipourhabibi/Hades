package config

// Server holds the HTTP/gRPC listener configuration.
type Server struct {
	ListenPort   int    `json:"listenPort" yaml:"listenPort"`
	RegistryHost string `json:"registryHost" yaml:"registryHost"`
	CertFile     string `json:"certFile" yaml:"certFile"`
	CertKey      string `json:"certKey" yaml:"certKey"`
	// EnableReflection serves the gRPC reflection API. It exposes the full
	// service and method schema without authentication, so it is off by default
	// and should stay off outside development.
	EnableReflection bool `json:"enableReflection" yaml:"enableReflection"`
}
