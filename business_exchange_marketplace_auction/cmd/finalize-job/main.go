package main

import (
	"log"
	"time"

	"auction_service/internal/config"
	"auction_service/internal/database"
	"auction_service/internal/logger"
	"auction_service/internal/models"
	"auction_service/internal/services"

	"go.uber.org/zap"
)

func main() {
	// 載入配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 初始化 logger
	logger, err := logger.New(cfg)
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	logger.Info("Starting auction finalize job")

	// 連接資料庫
	db, err := database.Connect(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}

	// 初始化服務
	auctionService := &services.AuctionService{DB: db, Logger: logger}
	notificationService := &services.NotificationService{DB: db, Logger: logger, Config: cfg}

	// 查找需要結束的拍賣
	var auctions []models.Auction
	now := time.Now()

	err = db.Where("status_code IN (?) AND (end_at <= ? OR (extended_until IS NOT NULL AND extended_until <= ?))",
		[]string{string(models.AuctionStatusActive), string(models.AuctionStatusExtended)},
		now, now).Find(&auctions).Error

	if err != nil {
		logger.Fatal("Failed to find auctions to finalize", zap.Error(err))
	}

	logger.Info("Found auctions to finalize", zap.Int("count", len(auctions)))

	for _, auction := range auctions {
		logger.Info("Finalizing auction", zap.Uint64("auction_id", auction.AuctionID))

		if err := auctionService.FinalizeAuction(auction.AuctionID); err != nil {
			logger.Error("Failed to finalize auction",
				zap.Uint64("auction_id", auction.AuctionID),
				zap.Error(err),
			)
			continue
		}

		// 發送通知
		if err := notificationService.SendAuctionEndNotifications(auction.AuctionID); err != nil {
			logger.Error("Failed to send notifications",
				zap.Uint64("auction_id", auction.AuctionID),
				zap.Error(err),
			)
		}

		logger.Info("Successfully finalized auction", zap.Uint64("auction_id", auction.AuctionID))
	}

	logger.Info("Finalize job completed", zap.Int("processed", len(auctions)))
}
