package casbin

import "gorm.io/gorm"

type CasbinStorage struct {
	db *gorm.DB
}

func New(db *gorm.DB) *CasbinStorage {
	return &CasbinStorage{
		db: db,
	}
}

func (c *CasbinStorage) GetDB() *gorm.DB {
	return c.db
}
