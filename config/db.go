package config

// DB is the config model for our database
type DB struct {
	ConnectionString string `json:"connectionString" yaml:"connectionString"`
}
