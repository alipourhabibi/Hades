package config

// DB holds the PostgreSQL connection parameters.
type DB struct {
	ConnectionString string `json:"connectionString" yaml:"connectionString"`
}
