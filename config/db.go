package config

type DB struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	DBName   string `json:"dbName" yaml:"dbName"`
	SslMode  bool   `json:"sslMode" yaml:"sslMode"`
}
