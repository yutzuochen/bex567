package services

import (
	"fmt"

	"auction_service/internal/config"
	"auction_service/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type NotificationService struct {
	DB     *gorm.DB
	Logger *zap.Logger
	Config *config.Config
}

// SendAuctionEndNotifications 發送拍賣結束通知
func (s *NotificationService) SendAuctionEndNotifications(auctionID uint64) error {
	// 取得拍賣資訊
	var auction models.Auction
	if err := s.DB.First(&auction, auctionID).Error; err != nil {
		return fmt.Errorf("failed to find auction %d: %w", auctionID, err)
	}

	// 取得所有有效出價並按排名排序
	var bids []models.Bid
	if err := s.DB.Where("auction_id = ? AND accepted = ? AND deleted_at IS NULL AND final_rank IS NOT NULL", 
		auctionID, true).Order("final_rank ASC").Find(&bids).Error; err != nil {
		return fmt.Errorf("failed to get ranked bids: %w", err)
	}

	s.Logger.Info("Preparing to send notifications",
		zap.Uint64("auction_id", auctionID),
		zap.Int("bid_count", len(bids)),
	)

	// 收集所有參與者ID
	participantIDs := make(map[uint64]bool)
	for _, bid := range bids {
		participantIDs[bid.BidderID] = true
	}

	// 發送賣家通知
	if err := s.queueNotification(auctionID, auction.SellerID, models.NotificationKindSellerResult); err != nil {
		s.Logger.Error("Failed to queue seller notification", zap.Error(err))
	}

	// 發送得標者通知（第1名）
	if len(bids) > 0 {
		winner := bids[0]
		if err := s.queueNotification(auctionID, winner.BidderID, models.NotificationKindWinner); err != nil {
			s.Logger.Error("Failed to queue winner notification", 
				zap.Uint64("bidder_id", winner.BidderID),
				zap.Error(err),
			)
		}
	}

	// 發送前7名通知（第2-7名）
	for i := 1; i < len(bids) && i < 7; i++ {
		bid := bids[i]
		if err := s.queueNotification(auctionID, bid.BidderID, models.NotificationKindTop7); err != nil {
			s.Logger.Error("Failed to queue top7 notification",
				zap.Uint64("bidder_id", bid.BidderID),
				zap.Int("rank", i+1),
				zap.Error(err),
			)
		}
	}

	// 發送其他參與者通知（第8名以後）
	for i := 7; i < len(bids); i++ {
		bid := bids[i]
		if err := s.queueNotification(auctionID, bid.BidderID, models.NotificationKindParticipantEnd); err != nil {
			s.Logger.Error("Failed to queue participant notification",
				zap.Uint64("bidder_id", bid.BidderID),
				zap.Int("rank", i+1),
				zap.Error(err),
			)
		}
	}

	s.Logger.Info("Queued all notifications",
		zap.Uint64("auction_id", auctionID),
		zap.Int("participants", len(participantIDs)),
	)

	return nil
}

// queueNotification 將通知加入佇列
func (s *NotificationService) queueNotification(auctionID, userID uint64, kind models.NotificationKind) error {
	// 檢查是否已經發送過相同通知
	var existing models.AuctionNotificationLog
	if err := s.DB.Where("auction_id = ? AND user_id = ? AND kind = ?", 
		auctionID, userID, kind).First(&existing).Error; err == nil {
		// 已存在，跳過
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing notification: %w", err)
	}

	// 創建通知記錄
	notification := &models.AuctionNotificationLog{
		AuctionID: auctionID,
		UserID:    userID,
		Kind:      kind,
		Channel:   models.NotificationChannelEmail, // 預設使用 email
		Status:    models.NotificationStatusQueued,
	}

	// 設定通知內容
	meta := map[string]interface{}{
		"auction_id": auctionID,
		"kind":       string(kind),
		"created_at": notification.CreatedAt,
	}

	switch kind {
	case models.NotificationKindWinner:
		meta["message"] = "恭喜您得標！"
		meta["title"] = "拍賣得標通知"
	case models.NotificationKindTop7:
		meta["message"] = "您在此次拍賣中排名前7名"
		meta["title"] = "拍賣結果通知"
	case models.NotificationKindParticipantEnd:
		meta["message"] = "感謝您參與此次拍賣"
		meta["title"] = "拍賣結束通知"
	case models.NotificationKindSellerResult:
		meta["message"] = "您的拍賣已結束"
		meta["title"] = "拍賣結束通知"
	}

	if err := notification.SetMeta(meta); err != nil {
		return fmt.Errorf("failed to set notification meta: %w", err)
	}

	if err := s.DB.Create(notification).Error; err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	// 記錄通知事件
	event := &models.AuctionEvent{
		AuctionID:   auctionID,
		EventType:   models.EventTypeNotified,
		ActorUserID: &userID,
	}
	event.SetPayload(map[string]interface{}{
		"notification_kind": string(kind),
		"user_id":          userID,
	})
	if err := s.DB.Create(event).Error; err != nil {
		s.Logger.Error("Failed to create notification event", zap.Error(err))
	}

	return nil
}

// ProcessNotificationQueue 處理通知佇列（實際發送）
func (s *NotificationService) ProcessNotificationQueue() error {
	// 取得待處理的通知
	var notifications []models.AuctionNotificationLog
	if err := s.DB.Where("status = ?", models.NotificationStatusQueued).
		Limit(100).Find(&notifications).Error; err != nil {
		return fmt.Errorf("failed to get queued notifications: %w", err)
	}

	s.Logger.Info("Processing notification queue", zap.Int("count", len(notifications)))

	for _, notification := range notifications {
		if err := s.sendNotification(&notification); err != nil {
			s.Logger.Error("Failed to send notification",
				zap.Uint64("notification_id", notification.ID),
				zap.Uint64("user_id", notification.UserID),
				zap.String("kind", string(notification.Kind)),
				zap.Error(err),
			)
			
			// 標記為失敗
			notification.MarkAsFailed()
			s.DB.Save(&notification)
		} else {
			// 標記為已發送
			notification.MarkAsSent()
			s.DB.Save(&notification)
		}
	}

	return nil
}

// sendNotification 實際發送通知（這裡是模擬實作）
func (s *NotificationService) sendNotification(notification *models.AuctionNotificationLog) error {
	// 這裡應該整合實際的通知服務（如 SendGrid, LINE, SMS 等）
	// 現在只是記錄日誌作為示範
	
	var meta map[string]interface{}
	if err := notification.GetMeta(&meta); err != nil {
		return fmt.Errorf("failed to get notification meta: %w", err)
	}

	s.Logger.Info("Sending notification (mock)",
		zap.Uint64("user_id", notification.UserID),
		zap.String("kind", string(notification.Kind)),
		zap.String("channel", string(notification.Channel)),
		zap.Any("meta", meta),
	)

	// 模擬發送延遲
	// time.Sleep(100 * time.Millisecond)

	return nil
}