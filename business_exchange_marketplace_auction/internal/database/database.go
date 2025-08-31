package database

import (
	"fmt"
	"time"

	"auction_service/internal/config"
	"auction_service/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	var logLevel logger.LogLevel
	if cfg.AppEnv == "development" {
		logLevel = logger.Info
	} else {
		logLevel = logger.Error
	}

	db, err := gorm.Open(mysql.Open(cfg.GetDBDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 設置連接池
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 測試連接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.AuctionStatusRef{},
		&models.Auction{},
		&models.Bid{},
		&models.AuctionStatusHistory{},
		&models.AuctionEvent{},
		&models.AuctionBidderAlias{},
		&models.AuctionBidHistogram{},
		&models.UserBlacklist{},
		&models.AuctionNotificationLog{},
		&models.AuctionStreamOffset{},
		&models.AuditLog{},
	)
}