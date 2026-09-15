package database

import (
	"fmt"
	"log"

	"github.com/mrhumster/comments-service/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupDatabase connects to Postgres. Schema is owned by db-migrate
// (target `comments`) — no AutoMigrate here, matching identity/stream/events.
func SetupDatabase(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.GetDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Raw("SELECT 1").Error; err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	log.Printf("database connected (%s)", cfg.Database.Name)
	return db, nil
}
