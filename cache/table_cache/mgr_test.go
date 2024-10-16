package table_cache

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const defaultDBPath = "../testdata/test.db"

func initDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(defaultDBPath), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	return db
}
