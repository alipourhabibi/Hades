package db

import (
	"context"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/casbin"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBs holds an instance of the storage/db packages that are needed by other packages
type DBs struct {
	UserStorage    *user.UserStorage
	SessionStorage *session.SessionStorage
	ModuleStorage  *module.ModuleStorage
	CasbinStorage  *casbin.CasbinStorage
	CommitStorage  *commit.CommitStorage
	UOW            UnitOfWork
}

// New creates an instance of DBs
func New(c config.DB, logger *log.LoggerWrapper) (*DBs, error) {
	ctx := context.Background()

	// Create a new connection pool with pgx
	config, err := pgxpool.ParseConfig(c.ConnectionString)
	if err != nil {
		return nil, err
	}

	// Establish a connection to the database using the pool
	pgxDB, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	uow := NewUnitOfWork(pgxDB)
	userStorage := user.New(pgxDB)
	sessionStorage := session.New(pgxDB)
	casbinStorage := casbin.New(pgxDB)
	moduleStorage := module.New(pgxDB)
	commitStorage := commit.New(pgxDB)

	return &DBs{
		UserStorage:    userStorage,
		SessionStorage: sessionStorage,
		ModuleStorage:  moduleStorage,
		CasbinStorage:  casbinStorage,
		CommitStorage:  commitStorage,
		UOW:            uow,
	}, nil
}
