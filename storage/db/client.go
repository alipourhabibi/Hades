package db

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DBs holds an instance of the storage/db packages that are needed by other packages
type DBs struct {
}

// New creates an instance of DBs
func New(c config.DB, logger *log.LoggerWrapper) (*DBs, error) {

	// Create the connection string based on the config values
	sslMode := "disable"
	if c.SslMode {
		sslMode = "enable"
	}
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d sslmode=%s",
		c.User, c.Password, c.DBName, c.Host, c.Port, sslMode)

	gormDB, err := gorm.Open(postgres.Open(connStr), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// it will be injected to all the db packages
	_ = gormDB

	return &DBs{}, nil
}
