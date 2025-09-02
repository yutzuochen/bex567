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
	Amount         float64  `json:"amount" binding:"required,gte=0"`
	ClientSeq      int64    `json:"client_seq" binding:"required"`
	MaxProxyAmount *float64 `json:"max_proxy_amount,omitempty" binding:"omitempty,gte=0"` // 英式拍賣代理出價上限
}

// PlaceBidResponse 出價響應
type PlaceBidResponse struct {
	Accepted       bool           `json:"accepted"`
	RejectReason   string         `json:"reject_reason,omitempty"`
	ServerTime     time.Time      `json:"server_time"`
	SoftClose      *SoftCloseInfo `json:"soft_close,omitempty"`
	EventID        uint64         `json:"event_id"`
	
	// 英式拍賣專用回應字段
	IsHighestBid   bool           `json:"is_highest_bid,omitempty"`
	CurrentPrice   *float64       `json:"current_price,omitempty"`
	MinimumNextBid *float64       `json:"minimum_next_bid,omitempty"`
	ReserveMet     *bool          `json:"reserve_met,omitempty"`
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

	// 檢查出價金額（根據拍賣類型）
	if auction.IsEnglishAuction() {
		if valid, reason := auction.ValidateEnglishBidAmount(req.Amount); !valid {
			tx.Rollback()
			var message string
			switch reason {
			case "bid_out_of_range":
				message = "Bid amount is out of allowed range"
			case "bid_under_minimum":
				minimumBid := auction.GetMinimumBid()
				message = "Bid must be at least " + strconv.FormatFloat(minimumBid, 'f', 2, 64)
			default:
				message = "Invalid bid amount"
			}
			
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"code":         reason,
				"message":      message,
				"current_price": auction.GetCurrentPriceForDisplay(),
				"minimum_bid":  auction.GetMinimumBid(),
			}})
			return
		}
	} else {
		// 密封拍賣的原有邏輯
		if !auction.ValidateBidAmount(req.Amount) {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"code":    "out_of_range",
				"message": "Bid amount is out of allowed range",
				"hint":    "Allowed range: " + strconv.FormatFloat(auction.AllowedMinBid, 'f', 2, 64) + " - " + strconv.FormatFloat(auction.AllowedMaxBid, 'f', 2, 64),
			}})
			return
		}
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
		AuctionID:      auctionID,
		BidderID:       userIDValue,
		Amount:         req.Amount,
		ClientSeq:      req.ClientSeq,
		SourceIPHash:   sourceIPHash[:],
		UserAgentHash:  userAgentHash[:],
		Accepted:       true,
		MaxProxyAmount: req.MaxProxyAmount,
	}

	// 英式拍賣設為可見，密封拍賣設為不可見直到結束
	if auction.IsEnglishAuction() {
		bid.IsVisible = true
	} else {
		bid.IsVisible = false
	}
	
	// 檢查是否為英式拍賣的最高出價
	isHighestBid := false
	if auction.IsEnglishAuction() {
		// 先將其他出價標記為非最高
		if err := tx.Model(&models.Bid{}).
			Where("auction_id = ? AND bid_id != ?", auctionID, 0).
			Update("is_winning", false).Error; err != nil {
			h.Logger.Error("Failed to update other bids", zap.Error(err))
		}
		
		isHighestBid = true
		bid.IsWinning = true
		
		// 更新拍賣的當前價格
		auction.UpdateCurrentPrice(req.Amount, userIDValue)
		if err := tx.Save(&auction).Error; err != nil {
			tx.Rollback()
			h.Logger.Error("Failed to update auction current price", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"code":    "internal_error",
				"message": "Failed to update auction",
			}})
			return
		}
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

	// 英式拍賣的額外回應資訊
	if auction.IsEnglishAuction() {
		response.IsHighestBid = isHighestBid
		response.CurrentPrice = auction.CurrentPrice
		minimumNext := auction.GetMinimumBid()
		response.MinimumNextBid = &minimumNext
		response.ReserveMet = &auction.ReserveMet
	}

	if softCloseInfo.Extended {
		response.SoftClose = softCloseInfo
	}

	// WebSocket 廣播出價事件
	if h.WSHandler != nil {
		if auction.IsEnglishAuction() {
			// 英式拍賣：廣播價格更新和出價接受事件
			priceData := map[string]interface{}{
				"current_price":    *auction.CurrentPrice,
				"highest_bidder":   userIDValue,
				"minimum_next_bid": auction.GetMinimumBid(),
				"reserve_met":      auction.ReserveMet,
				"event_id":         event.EventID,
				"server_time":      response.ServerTime,
			}
			
			h.WSHandler.Hub.BroadcastToAuction(
				auctionID,
				websocket.MessageTypePriceChanged,
				priceData,
			)
			
			// 如果剛達到保留價，發送特殊事件
			if auction.ReserveMet && auction.ReservePrice != nil && req.Amount >= *auction.ReservePrice {
				reserveData := map[string]interface{}{
					"reserve_price": *auction.ReservePrice,
					"current_price": *auction.CurrentPrice,
					"event_id":      event.EventID,
					"server_time":   response.ServerTime,
				}
				
				h.WSHandler.Hub.BroadcastToAuction(
					auctionID,
					websocket.MessageTypeReserveMet,
					reserveData,
				)
			}
		} else {
			// 密封拍賣：原有邏輯
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
		}

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

// BuyItNowRequest 直購請求
type BuyItNowRequest struct {
	ClientSeq int64 `json:"client_seq" binding:"required"`
}

// BuyItNowResponse 直購響應
type BuyItNowResponse struct {
	Success      bool      `json:"success"`
	FinalPrice   float64   `json:"final_price"`
	AuctionEnded bool      `json:"auction_ended"`
	ServerTime   time.Time `json:"server_time"`
	EventID      uint64    `json:"event_id"`
}

// BuyItNow 直購商品 POST /api/v1/auctions/:id/buy-now
func (h *BidHandler) BuyItNow(c *gin.Context) {
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

	var req BuyItNowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid request format",
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
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to find auction",
		}})
		return
	}

	// 檢查是否支持直購
	if !auction.CanBuyItNow() {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "buy_it_now_not_available",
			"message": "Buy it now is not available for this auction",
		}})
		return
	}

	// 執行直購
	if !auction.ExecuteBuyItNow(userIDValue) {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "buy_it_now_failed",
			"message": "Failed to execute buy it now",
		}})
		return
	}

	// 保存拍賣狀態
	if err := tx.Save(&auction).Error; err != nil {
		tx.Rollback()
		h.Logger.Error("Failed to save auction after buy it now", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to complete purchase",
		}})
		return
	}

	// 創建直購記錄（作為特殊的出價）
	sourceIPHash := sha256.Sum256([]byte(c.ClientIP()))
	userAgentHash := sha256.Sum256([]byte(c.GetHeader("User-Agent")))

	bid := &models.Bid{
		AuctionID:     auctionID,
		BidderID:      userIDValue,
		Amount:        *auction.BuyItNow,
		ClientSeq:     req.ClientSeq,
		SourceIPHash:  sourceIPHash[:],
		UserAgentHash: userAgentHash[:],
		Accepted:      true,
		IsWinning:     true,
		IsVisible:     true,
	}

	if err := tx.Create(bid).Error; err != nil {
		tx.Rollback()
		h.Logger.Error("Failed to create buy it now bid", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to complete purchase",
		}})
		return
	}

	// 記錄狀態歷史
	history := &models.AuctionStatusHistory{
		AuctionID:  auctionID,
		FromStatus: string(models.AuctionStatusActive),
		ToStatus:   string(models.AuctionStatusEnded),
		Reason:     "Buy it now executed",
		OperatorID: &userIDValue,
	}
	if err := tx.Create(history).Error; err != nil {
		h.Logger.Error("Failed to create status history", zap.Error(err))
	}

	// 創建事件記錄
	event := &models.AuctionEvent{
		AuctionID:   auctionID,
		EventType:   models.EventTypeClosed,
		ActorUserID: &userIDValue,
	}
	if err := tx.Create(event).Error; err != nil {
		tx.Rollback()
		h.Logger.Error("Failed to create buy it now event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to complete purchase",
		}})
		return
	}

	// 審計日誌
	auditLog := models.NewAuditLog(
		&userIDValue,
		models.ActionBidPlace,
		models.EntityTypeBid,
		bid.BidID,
		map[string]interface{}{
			"type":        "buy_it_now",
			"amount":      *auction.BuyItNow,
			"auction_id":  auctionID,
		},
	)
	if err := tx.Create(auditLog).Error; err != nil {
		h.Logger.Error("Failed to create audit log", zap.Error(err))
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		h.Logger.Error("Failed to commit buy it now transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to complete purchase",
		}})
		return
	}

	response := BuyItNowResponse{
		Success:      true,
		FinalPrice:   *auction.BuyItNow,
		AuctionEnded: true,
		ServerTime:   time.Now(),
		EventID:      event.EventID,
	}

	// WebSocket 廣播拍賣結束事件
	if h.WSHandler != nil {
		closeData := map[string]interface{}{
			"reason":      "buy_it_now",
			"winner_id":   userIDValue,
			"final_price": *auction.BuyItNow,
			"ended_at":    response.ServerTime,
			"event_id":    event.EventID,
		}

		h.WSHandler.Hub.BroadcastToAuction(
			auctionID,
			websocket.MessageTypeClosed,
			closeData,
		)
	}

	c.JSON(http.StatusOK, response)
}
