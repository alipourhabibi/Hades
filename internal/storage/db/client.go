package db

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/models"
	"github.com/alipourhabibi/Hades/internal/storage/db/casbin"
	"github.com/alipourhabibi/Hades/internal/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DBs holds an instance of the storage/db packages that are needed by other packages
type DBs struct {
	gormDB         *gorm.DB
	UserStorage    *user.UserStorage
	SessionStorage *session.SessionStorage
	ModuleStorage  *module.ModuleStorage
	CasbinStorage  *casbin.CasbinStorage
	CommitStorage  *commit.CommitStorage
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

	gormDB, err := gorm.Open(postgres.Open(connStr), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	if c.Debug {
		gormDB = gormDB.Debug()
	}

	userStorage := user.New(gormDB)
	sessionStorage := session.New(gormDB)
	casbinStorage := casbin.New(gormDB)
	moduleStorage := module.New(gormDB)
	commitStorage := commit.New(gormDB)

	return &DBs{
		gormDB:         gormDB,
		UserStorage:    userStorage,
		SessionStorage: sessionStorage,
		ModuleStorage:  moduleStorage,
		CasbinStorage:  casbinStorage,
		CommitStorage:  commitStorage,
	}, nil
}

// AutoMigrate will migrate the dbs
func (d *DBs) AutoMigrate() error {
	var err error
	err = d.gormDB.AutoMigrate(&models.User{})
	if err != nil {
		return err
	}

	err = d.gormDB.AutoMigrate(&models.Session{})
	if err != nil {
		return err
	}

	err = d.gormDB.AutoMigrate(&models.Commit{})
	if err != nil {
		return err
	}

	err = d.gormDB.AutoMigrate(&models.Module{})
	if err != nil {
		return err
	}

	return nil
}
