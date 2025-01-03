package config

type Server struct {
	ListenPort int    `json:"listenPort" yaml:"listenPort"`
	CertFile   string `json:"certFile" yaml:"certFile"`
	CertKey    string `json:"certKey" yaml:"certKey"`
}
