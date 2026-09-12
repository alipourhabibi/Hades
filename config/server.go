package config

import "time"

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

	// TrustedProxies lists CIDR ranges (or bare IPs) whose X-Forwarded-For and
	// X-Real-IP headers are honoured when determining the client IP address.
	//
	// The client IP drives login, registration and password-reset rate limiting
	// and is recorded in the audit log, and forwarding headers are trivially
	// forged. Leave this empty unless Hades sits behind a reverse proxy you
	// control: with no entries the transport peer address is always used.
	TrustedProxies []string `json:"trustedProxies" yaml:"trustedProxies"`

	// ReadHeaderTimeout bounds how long a client may take to send request
	// headers. Zero uses a built-in default; see internal/hades/run.go.
	ReadHeaderTimeout time.Duration `json:"readHeaderTimeout" yaml:"readHeaderTimeout"`
	// ReadTimeout bounds how long a client may take to send a full request.
	// Zero uses a built-in default.
	ReadTimeout time.Duration `json:"readTimeout" yaml:"readTimeout"`
	// IdleTimeout bounds how long a keep-alive connection may sit unused.
	// Zero uses a built-in default.
	IdleTimeout time.Duration `json:"idleTimeout" yaml:"idleTimeout"`
	// ShutdownTimeout bounds how long in-flight requests are given to finish
	// when the server is stopping. Zero uses a built-in default.
	ShutdownTimeout time.Duration `json:"shutdownTimeout" yaml:"shutdownTimeout"`
}
