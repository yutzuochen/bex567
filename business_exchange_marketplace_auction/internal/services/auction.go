package services

import (
	"fmt"
	"time"

	"auction_service/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuctionService struct {
	DB     *gorm.DB
	Logger *zap.Logger
}

// FinalizeAuction 結束拍賣並計算排名
func (s *AuctionService) FinalizeAuction(auctionID uint64) error {
	// 開始事務
	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 鎖定拍賣記錄
	var auction models.Auction
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&auction, auctionID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find auction %d: %w", auctionID, err)
	}

	// 檢查拍賣狀態
	if auction.StatusCode == string(models.AuctionStatusEnded) ||
		auction.StatusCode == string(models.AuctionStatusCancelled) {
		tx.Rollback()
		return fmt.Errorf("auction %d already finalized with status: %s", auctionID, auction.StatusCode)
	}

	// 取得所有有效的出價並排序
	var bids []models.Bid
	if err := tx.Where("auction_id = ? AND accepted = ? AND deleted_at IS NULL", 
		auctionID, true).Order("amount DESC, created_at ASC").Find(&bids).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get bids for auction %d: %w", auctionID, err)
	}

	s.Logger.Info("Found valid bids for auction",
		zap.Uint64("auction_id", auctionID),
		zap.Int("bid_count", len(bids)),
	)

	// 更新出價排名
	for i, bid := range bids {
		rank := i + 1
		bid.FinalRank = &rank
		if err := tx.Save(&bid).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update bid rank for bid %d: %w", bid.BidID, err)
		}
	}

	// 更新拍賣狀態
	oldStatus := auction.StatusCode
	auction.StatusCode = string(models.AuctionStatusEnded)
	
	if err := tx.Save(&auction).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update auction status: %w", err)
	}

	// 記錄狀態歷史
	history := &models.AuctionStatusHistory{
		AuctionID:  auctionID,
		FromStatus: oldStatus,
		ToStatus:   auction.StatusCode,
		Reason:     "Finalized by system job",
	}
	if err := tx.Create(history).Error; err != nil {
		s.Logger.Error("Failed to create status history", zap.Error(err))
	}

	// 記錄結束事件
	event := &models.AuctionEvent{
		AuctionID: auctionID,
		EventType: models.EventTypeClosed,
	}
	event.SetPayload(map[string]interface{}{
		"ended_at":    time.Now(),
		"winner_rank": func() int {
			if len(bids) > 0 {
				return 1
			}
			return 0
		}(),
		"total_bids": len(bids),
	})
	if err := tx.Create(event).Error; err != nil {
		s.Logger.Error("Failed to create close event", zap.Error(err))
	}

	// 記錄審計日誌
	auditLog := models.NewAuditLog(
		nil, // System action, no user
		models.ActionAuctionClose,
		models.EntityTypeAuction,
		auctionID,
		map[string]interface{}{
			"before_status": oldStatus,
			"after_status":  auction.StatusCode,
			"total_bids":    len(bids),
		},
	)
	if err := tx.Create(auditLog).Error; err != nil {
		s.Logger.Error("Failed to create audit log", zap.Error(err))
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit finalize transaction: %w", err)
	}

	s.Logger.Info("Successfully finalized auction",
		zap.Uint64("auction_id", auctionID),
		zap.Int("total_bids", len(bids)),
		zap.String("old_status", oldStatus),
		zap.String("new_status", auction.StatusCode),
	)

	return nil
}

// ComputeBidHistogram 計算出價分佈圖
func (s *AuctionService) ComputeBidHistogram(auctionID uint64) error {
	// 取得拍賣資訊
	var auction models.Auction
	if err := s.DB.First(&auction, auctionID).Error; err != nil {
		return fmt.Errorf("failed to find auction %d: %w", auctionID, err)
	}

	// 計算桶的大小和範圍
	bucketCount := 10
	priceRange := auction.AllowedMaxBid - auction.AllowedMinBid
	bucketSize := priceRange / float64(bucketCount)

	// 取得所有有效出價
	var bids []models.Bid
	if err := s.DB.Where("auction_id = ? AND accepted = ? AND deleted_at IS NULL", 
		auctionID, true).Find(&bids).Error; err != nil {
		return fmt.Errorf("failed to get bids: %w", err)
	}

	// 計算每個桶的出價數量
	buckets := make(map[int]int)
	for _, bid := range bids {
		bucketIndex := int((bid.Amount - auction.AllowedMinBid) / bucketSize)
		if bucketIndex >= bucketCount {
			bucketIndex = bucketCount - 1
		}
		buckets[bucketIndex]++
	}

	// 清除舊的分佈記錄（保持最新5次）
	s.DB.Where("auction_id = ?", auctionID).
		Order("computed_at DESC").
		Offset(5).
		Delete(&models.AuctionBidHistogram{})

	// 儲存新的分佈記錄
	computedAt := time.Now()
	for i := 0; i < bucketCount; i++ {
		low := auction.AllowedMinBid + float64(i)*bucketSize
		high := low + bucketSize
		count := buckets[i]

		histogram := &models.AuctionBidHistogram{
			AuctionID:   auctionID,
			BucketLow:   low,
			BucketHigh:  high,
			BidCount:    count,
			ComputedAt:  computedAt,
		}

		if err := s.DB.Create(histogram).Error; err != nil {
			s.Logger.Error("Failed to create histogram record",
				zap.Uint64("auction_id", auctionID),
				zap.Error(err),
			)
		}
	}

	s.Logger.Info("Computed bid histogram",
		zap.Uint64("auction_id", auctionID),
		zap.Int("total_bids", len(bids)),
		zap.Int("buckets", bucketCount),
	)

	return nil
}