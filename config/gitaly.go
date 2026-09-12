package config

// Gitaly holds the connection parameters for the Gitaly gRPC server.
type Gitaly struct {
	Host               string `json:"host" yaml:"host"`
	Port               int    `json:"port" yaml:"port"`
	DefaultStorageName string `json:"defaultStorageName" yaml:"defaultStorageName"`

	// TLS enables transport security on the Gitaly connection. It defaults to
	// false because the shipped deployment runs Gitaly on a private network,
	// but a deployment where the two are not co-located must be able to turn it
	// on without editing code.
	TLS bool `json:"tls" yaml:"tls"`

	// CACertFile is an optional PEM bundle used to verify the Gitaly server
	// certificate. Empty means the system pool.
	CACertFile string `json:"caCertFile" yaml:"caCertFile"`

	// ServerNameOverride sets the TLS server name when it differs from Host,
	// which happens when Gitaly is reached through a service address.
	ServerNameOverride string `json:"serverNameOverride" yaml:"serverNameOverride"`
}
