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

	// 首先自動啟用已過開始時間的 draft 拍賣
	now := time.Now()
	var draftAuctions []models.Auction
	err = db.Where("status_code = ? AND start_at <= ?", 
		string(models.AuctionStatusDraft), now).Find(&draftAuctions).Error
	
	if err != nil {
		logger.Error("Failed to find draft auctions to activate", zap.Error(err))
	} else {
		logger.Info("Found draft auctions to activate", zap.Int("count", len(draftAuctions)))
		
		for _, auction := range draftAuctions {
			logger.Info("Auto-activating auction", zap.Uint64("auction_id", auction.AuctionID))
			
			// 更新拍賣狀態為 active
			auction.StatusCode = string(models.AuctionStatusActive)
			if err := db.Save(&auction).Error; err != nil {
				logger.Error("Failed to activate auction",
					zap.Uint64("auction_id", auction.AuctionID),
					zap.Error(err),
				)
				continue
			}
			
			// 記錄狀態歷史
			history := &models.AuctionStatusHistory{
				AuctionID:  auction.AuctionID,
				FromStatus: string(models.AuctionStatusDraft),
				ToStatus:   string(models.AuctionStatusActive),
				Reason:     "Auto-activated (start time reached)",
			}
			db.Create(history)
			
			logger.Info("Successfully activated auction", zap.Uint64("auction_id", auction.AuctionID))
		}
	}

	// 查找需要結束的拍賣
	var auctions []models.Auction

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
