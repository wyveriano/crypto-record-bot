package persistence

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB opens the SQLite database at the specified path and performs auto-migration.
func NewDB(dbPath string) (*gorm.DB, error) {
	if dbPath == "" {
		dbPath = "crypto_record.db"
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(dbPath), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sqlite database at %q: %w", dbPath, err)
	}

	if err := db.AutoMigrate(&AlertDAO{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate database schema: %w", err)
	}

	return db, nil
}
