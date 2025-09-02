package handlers

import (
	"crypto/sha256"
	"net/http"
	"strconv"
	"time"

	"auction_service/internal/middleware"
	"auction_service/internal/models"
	"auction_service/internal/websocket"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type BidHandler struct {
	DB        *gorm.DB
	Logger    *zap.Logger
	WSHandler *websocket.Handler
}

// PlaceBidRequest 出價請求
type PlaceBidRequest struct {
	Amount    float64 `json:"amount" binding:"required,gte=0"`
	ClientSeq int64   `json:"client_seq" binding:"required"`
}

// PlaceBidResponse 出價響應
type PlaceBidResponse struct {
	Accepted     bool           `json:"accepted"`
	RejectReason string         `json:"reject_reason,omitempty"`
	ServerTime   time.Time      `json:"server_time"`
	SoftClose    *SoftCloseInfo `json:"soft_close,omitempty"`
	EventID      uint64         `json:"event_id"`
}

// SoftCloseInfo 軟關閉資訊
type SoftCloseInfo struct {
	Extended      bool       `json:"extended"`
	ExtendedUntil *time.Time `json:"extended_until,omitempty"`
}

// PlaceBid 提交出價 POST /api/v1/auctions/:id/bids
func (h *BidHandler) PlaceBid(c *gin.Context) {
	auctionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid auction ID",
		}})
		return
	}

	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code":    "unauthorized",
			"message": "User not authenticated",
		}})
		return
	}
	userIDValue := userID.(uint64)

	var req PlaceBidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid request format",
			"details": err.Error(),
		}})
		return
	}

	// 檢查黑名單
	var blacklist models.UserBlacklist
	if err := h.DB.Where("user_id = ? AND is_active = ?", userIDValue, true).First(&blacklist).Error; err == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"code":    "blacklisted",
			"message": "User is blacklisted",
		}})
		return
	} else if err != gorm.ErrRecordNotFound {
		h.Logger.Error("Failed to check blacklist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to check user status",
		}})
		return
	}

	// 檢查出價頻率（5秒內最多1次）
	var recentBid models.Bid
	fiveSecondsAgo := time.Now().Add(-5 * time.Second)
	if err := h.DB.Where("auction_id = ? AND bidder_id = ? AND created_at > ?",
		auctionID, userIDValue, fiveSecondsAgo).First(&recentBid).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":             "too_frequent",
			"message":          "Bidding too frequently, please wait",
			"cooldown_seconds": 5,
		}})
		return
	}
	h.Logger.Info("start transaction!")
	// 開始事務
	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 鎖定拍賣記錄
	var auction models.Auction
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&auction, auctionID).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
				"code":    "not_found",
				"message": "Auction not found",
			}})
			return
		}
		h.Logger.Error("Failed to find auction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to find auction",
		}})
		return
	}

	// 檢查拍賣狀態
	if !auction.IsActive() {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "auction_not_active",
			"message": "Auction is not active",
		}})
		return
	}

	// 檢查是否已過截止時間
	effectiveEndTime := auction.GetEffectiveEndTime()
	if time.Now().After(effectiveEndTime) {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "past_deadline",
			"message": "Auction has ended",
		}})
		return
	}

	// 檢查出價金額範圍
	if !auction.ValidateBidAmount(req.Amount) {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "out_of_range",
			"message": "Bid amount is out of allowed range",
			"hint":    "Allowed range: " + strconv.FormatFloat(auction.AllowedMinBid, 'f', 2, 64) + " - " + strconv.FormatFloat(auction.AllowedMaxBid, 'f', 2, 64),
		}})
		return
	}

	// 檢查冪等性
	var existingBid models.Bid
	if err := tx.Where("auction_id = ? AND bidder_id = ? AND client_seq = ?",
		auctionID, userIDValue, req.ClientSeq).First(&existingBid).Error; err == nil {
		tx.Rollback()
		c.JSON(http.StatusOK, PlaceBidResponse{
			Accepted:     existingBid.Accepted,
			RejectReason: existingBid.RejectReason,
			ServerTime:   time.Now(),
			EventID:      0, // 原有記錄不產生新事件
		})
		return
	}

	// 檢查軟關閉並延長時間
	softCloseInfo := &SoftCloseInfo{Extended: false}
	if auction.CanExtend() {
		auction.ExtendAuction()
		if err := tx.Save(&auction).Error; err != nil {
			tx.Rollback()
			h.Logger.Error("Failed to extend auction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"code":    "internal_error",
				"message": "Failed to process bid",
			}})
			return
		}

		// 記錄狀態歷史
		history := &models.AuctionStatusHistory{
			AuctionID:  auctionID,
			FromStatus: string(models.AuctionStatusActive),
			ToStatus:   auction.StatusCode,
			Reason:     "Extended due to bid in soft-close window",
		}
		if err := tx.Create(history).Error; err != nil {
			h.Logger.Error("Failed to create status history", zap.Error(err))
		}

		softCloseInfo.Extended = true
		softCloseInfo.ExtendedUntil = auction.ExtendedUntil
	}

	// 創建出價記錄
	sourceIPHash := sha256.Sum256([]byte(c.ClientIP()))
	userAgentHash := sha256.Sum256([]byte(c.GetHeader("User-Agent")))

	bid := &models.Bid{
		AuctionID:     auctionID,
		BidderID:      userIDValue,
		Amount:        req.Amount,
		ClientSeq:     req.ClientSeq,
		SourceIPHash:  sourceIPHash[:],
		UserAgentHash: userAgentHash[:],
		Accepted:      true,
	}

	if err := tx.Create(bid).Error; err != nil {
		tx.Rollback()
		h.Logger.Error("Failed to create bid", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to place bid",
		}})
		return
	}

	// 創建匿名別名（如果需要）
	if auction.IsAnonymous {
		var alias models.AuctionBidderAlias
		if err := tx.Where("auction_id = ? AND bidder_id = ?", auctionID, userIDValue).First(&alias).Error; err == gorm.ErrRecordNotFound {
			// 計算新的別名編號
			var maxAliasNum int
			tx.Model(&models.AuctionBidderAlias{}).
				Where("auction_id = ?", auctionID).
				Select("COALESCE(MAX(alias_num), 0)").Scan(&maxAliasNum)

			alias = models.AuctionBidderAlias{
				AuctionID:  auctionID,
				BidderID:   userIDValue,
				AliasNum:   maxAliasNum + 1,
				AliasLabel: "Bidder #" + strconv.Itoa(maxAliasNum+1),
			}
			if err := tx.Create(&alias).Error; err != nil {
				h.Logger.Error("Failed to create bidder alias", zap.Error(err))
			}
		}
	}

	// 創建事件記錄
	event := &models.AuctionEvent{
		AuctionID:   auctionID,
		EventType:   models.EventTypeBidAccepted,
		ActorUserID: &userIDValue,
	}
	if err := tx.Create(event).Error; err != nil {
		tx.Rollback()
		h.Logger.Error("Failed to create auction event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to place bid",
		}})
		return
	}

	// 創建軟關閉延長事件（如果有延長）
	if softCloseInfo.Extended {
		extendEvent := &models.AuctionEvent{
			AuctionID: auctionID,
			EventType: models.EventTypeExtended,
		}
		extendEvent.SetPayload(map[string]interface{}{
			"extended_until":  auction.ExtendedUntil,
			"extension_count": auction.ExtensionCount,
		})
		if err := tx.Create(extendEvent).Error; err != nil {
			h.Logger.Error("Failed to create extend event", zap.Error(err))
		}
	}

	// 記錄審計日誌
	auditLog := models.NewAuditLog(
		&userIDValue,
		models.ActionBidPlace,
		models.EntityTypeBid,
		bid.BidID,
		bid,
	)
	if err := tx.Create(auditLog).Error; err != nil {
		h.Logger.Error("Failed to create audit log", zap.Error(err))
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		h.Logger.Error("Failed to commit bid transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to place bid",
		}})
		return
	}

	response := PlaceBidResponse{
		Accepted:   true,
		ServerTime: time.Now(),
		EventID:    event.EventID,
	}

	if softCloseInfo.Extended {
		response.SoftClose = softCloseInfo
	}

	// WebSocket 廣播出價事件
	if h.WSHandler != nil {
		bidData := map[string]interface{}{
			"amount":      bid.Amount,
			"accepted":    true,
			"event_id":    event.EventID,
			"server_time": response.ServerTime,
		}

		// 向除了出價者以外的所有參與者廣播
		h.WSHandler.Hub.BroadcastToAuction(
			auctionID,
			websocket.MessageTypeBidAccepted,
			bidData,
		)

		// 如果有軟關閉延長，廣播延長事件
		if softCloseInfo.Extended {
			extendData := map[string]interface{}{
				"extended":        true,
				"extended_until":  auction.ExtendedUntil,
				"extension_count": auction.ExtensionCount,
				"event_id":        event.EventID + 1, // 延長事件ID
			}

			h.WSHandler.Hub.BroadcastToAuction(
				auctionID,
				websocket.MessageTypeExtended,
				extendData,
			)
		}
	}

	c.JSON(http.StatusOK, response)
}
