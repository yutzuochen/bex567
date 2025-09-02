package handlers

import (
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

type AuctionHandler struct {
	DB        *gorm.DB
	Logger    *zap.Logger
	WSHandler *websocket.Handler
}

// CreateAuctionRequest 創建拍賣請求
type CreateAuctionRequest struct {
	ListingID       uint64    `json:"listing_id" binding:"required"`
	AuctionType     string    `json:"auction_type" binding:"required,oneof=sealed english dutch"`
	AllowedMinBid   float64   `json:"allowed_min_bid" binding:"required,gte=0"`
	AllowedMaxBid   float64   `json:"allowed_max_bid" binding:"required,gt=0"`
	StartAt         time.Time `json:"start_at" binding:"required"`
	EndAt           time.Time `json:"end_at" binding:"required"`
	IsAnonymous     bool      `json:"is_anonymous"`
	
	// 英式拍賣專用字段
	ReservePrice *float64 `json:"reserve_price,omitempty" binding:"omitempty,gte=0"`
	MinIncrement *float64 `json:"min_increment,omitempty" binding:"omitempty,gt=0"`
	BuyItNow     *float64 `json:"buy_it_now,omitempty" binding:"omitempty,gt=0"`
}

// CreateAuctionResponse 創建拍賣響應
type CreateAuctionResponse struct {
	AuctionID           uint64 `json:"auction_id"`
	StatusCode          string `json:"status_code"`
	SoftCloseTriggerSec int    `json:"soft_close_trigger_sec"`
	SoftCloseExtendSec  int    `json:"soft_close_extend_sec"`
}

// CreateAuction 創建拍賣 POST /api/v1/auctions
func (h *AuctionHandler) CreateAuction(c *gin.Context) {
	requestID := c.GetString("request_id")
	clientIP := c.ClientIP()
	
	h.Logger.Info("Creating new auction",
		zap.String("request_id", requestID),
		zap.String("client_ip", clientIP),
	)

	var req CreateAuctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("Invalid auction creation request format",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid request format",
			"details": err.Error(),
		}})
		return
	}

	h.Logger.Debug("Auction creation request parsed",
		zap.String("request_id", requestID),
		zap.Uint64("listing_id", req.ListingID),
		zap.Float64("min_bid", req.AllowedMinBid),
		zap.Float64("max_bid", req.AllowedMaxBid),
		zap.Time("start_at", req.StartAt),
		zap.Time("end_at", req.EndAt),
		zap.Bool("is_anonymous", req.IsAnonymous),
	)

	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.Logger.Warn("User not authenticated for auction creation",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code":    "unauthorized",
			"message": "User not authenticated",
		}})
		return
	}

	userIDValue := userID.(uint64)
	h.Logger.Info("User authenticated for auction creation",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
	)

	// 驗證業務規則
	h.Logger.Debug("Validating auction business rules",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
	)

	if req.AllowedMaxBid <= req.AllowedMinBid {
		h.Logger.Warn("Invalid bid range",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userIDValue),
			zap.Float64("min_bid", req.AllowedMinBid),
			zap.Float64("max_bid", req.AllowedMaxBid),
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code":    "invalid_range",
			"message": "Maximum bid must be greater than minimum bid",
		}})
		return
	}

	duration := req.EndAt.Sub(req.StartAt).Hours() / 24
	if duration < 1 || duration > 61 {
		h.Logger.Warn("Invalid auction duration",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userIDValue),
			zap.Float64("duration_days", duration),
			zap.Time("start_at", req.StartAt),
			zap.Time("end_at", req.EndAt),
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code":    "duration_exceeded",
			"message": "Auction duration must be between 1 and 61 days",
		}})
		return
	}

	if req.StartAt.Before(time.Now()) {
		h.Logger.Warn("Start time in the past",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userIDValue),
			zap.Time("start_at", req.StartAt),
			zap.Time("current_time", time.Now()),
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code":    "start_in_past",
			"message": "Start time cannot be in the past",
		}})
		return
	}

	h.Logger.Debug("Business rules validation passed",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Float64("duration_days", duration),
	)

	h.Logger.Debug("Creating auction model",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
	)

	// 設置拍賣類型
	var auctionType models.AuctionType
	switch req.AuctionType {
	case "english":
		auctionType = models.AuctionTypeEnglish
	case "dutch":
		auctionType = models.AuctionTypeDutch
	default:
		auctionType = models.AuctionTypeSealed
	}

	auction := &models.Auction{
		ListingID:       req.ListingID,
		SellerID:        userID.(uint64),
		AuctionType:     auctionType,
		StatusCode:      string(models.AuctionStatusDraft),
		AllowedMinBid:   req.AllowedMinBid,
		AllowedMaxBid:   req.AllowedMaxBid,
		StartAt:         req.StartAt,
		EndAt:           req.EndAt,
		IsAnonymous:     req.IsAnonymous,
		ReservePrice:    req.ReservePrice,
		BuyItNow:        req.BuyItNow,
	}

	// 設置英式拍賣的最小加價幅度
	if req.MinIncrement != nil {
		auction.MinIncrement = *req.MinIncrement
	} else if auctionType == models.AuctionTypeEnglish {
		auction.MinIncrement = 10000.00 // 預設最小加價
	}

	h.Logger.Debug("Saving auction to database",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("listing_id", auction.ListingID),
	)

	if err := h.DB.Create(auction).Error; err != nil {
		h.Logger.Error("Failed to create auction",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userIDValue),
			zap.Uint64("listing_id", req.ListingID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to create auction",
		}})
		return
	}

	h.Logger.Info("Auction created successfully",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("auction_id", auction.AuctionID),
		zap.Uint64("listing_id", auction.ListingID),
		zap.String("status", auction.StatusCode),
	)

	// 記錄審計日誌
	h.Logger.Debug("Creating audit log",
		zap.String("request_id", requestID),
		zap.Uint64("auction_id", auction.AuctionID),
	)

	auditLog := models.NewAuditLog(
		&userIDValue,
		models.ActionAuctionCreate,
		models.EntityTypeAuction,
		auction.AuctionID,
		auction,
	)
	
	if err := h.DB.Create(auditLog).Error; err != nil {
		h.Logger.Warn("Failed to create audit log",
			zap.String("request_id", requestID),
			zap.Uint64("auction_id", auction.AuctionID),
			zap.Error(err),
		)
	} else {
		h.Logger.Debug("Audit log created",
			zap.String("request_id", requestID),
			zap.Uint64("auction_id", auction.AuctionID),
		)
	}

	h.Logger.Info("Auction creation completed successfully",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("auction_id", auction.AuctionID),
	)

	c.JSON(http.StatusCreated, gin.H{
		"data": auction,
	})
}

// ActivateAuction 啟用拍賣 POST /api/v1/auctions/:id:activate
func (h *AuctionHandler) ActivateAuction(c *gin.Context) {
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

	var auction models.Auction
	if err := h.DB.First(&auction, auctionID).Error; err != nil {
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

	// 檢查權限
	if auction.SellerID != userID.(uint64) {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"code":    "forbidden",
			"message": "Only seller can activate auction",
		}})
		return
	}

	// 檢查狀態
	if auction.StatusCode != string(models.AuctionStatusDraft) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "invalid_state",
			"message": "Only draft auctions can be activated",
		}})
		return
	}

	// 更新狀態
	oldStatus := auction.StatusCode
	auction.StatusCode = string(models.AuctionStatusActive)
	
	if err := h.DB.Save(&auction).Error; err != nil {
		h.Logger.Error("Failed to activate auction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to activate auction",
		}})
		return
	}

	// 記錄狀態歷史
	userIDValue2 := userID.(uint64)
	history := &models.AuctionStatusHistory{
		AuctionID:  auctionID,
		FromStatus: oldStatus,
		ToStatus:   auction.StatusCode,
		Reason:     "Activated by seller",
		OperatorID: &userIDValue2,
	}
	h.DB.Create(history)

	// 記錄事件
	userIDValue3 := userID.(uint64)
	event := &models.AuctionEvent{
		AuctionID:   auctionID,
		EventType:   models.EventTypeOpen,
		ActorUserID: &userIDValue3,
	}
	h.DB.Create(event)

	// WebSocket 廣播拍賣啟用事件
	if h.WSHandler != nil {
		stateData := map[string]interface{}{
			"status_code": auction.StatusCode,
			"start_at":    auction.StartAt.Format(time.RFC3339),
			"end_at":      auction.EndAt.Format(time.RFC3339),
		}
		
		h.WSHandler.Hub.BroadcastToAuction(
			auctionID,
			websocket.MessageTypeState,
			stateData,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"status_code": auction.StatusCode,
		"start_at":    auction.StartAt.Format(time.RFC3339),
		"end_at":      auction.EndAt.Format(time.RFC3339),
	})
}

// CancelAuction 取消拍賣 POST /api/v1/auctions/:id:cancel
func (h *AuctionHandler) CancelAuction(c *gin.Context) {
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

	var auction models.Auction
	if err := h.DB.First(&auction, auctionID).Error; err != nil {
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

	// 檢查權限
	userRole, _ := c.Get(middleware.UserRoleKey)
	if auction.SellerID != userID.(uint64) && userRole != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"code":    "forbidden",
			"message": "Only seller or admin can cancel auction",
		}})
		return
	}

	// 檢查狀態
	if auction.StatusCode == string(models.AuctionStatusEnded) ||
		auction.StatusCode == string(models.AuctionStatusCancelled) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "invalid_state",
			"message": "Cannot cancel ended or already cancelled auction",
		}})
		return
	}

	// 更新狀態
	oldStatus := auction.StatusCode
	auction.StatusCode = string(models.AuctionStatusCancelled)
	
	if err := h.DB.Save(&auction).Error; err != nil {
		h.Logger.Error("Failed to cancel auction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to cancel auction",
		}})
		return
	}

	// 記錄狀態歷史
	reason := "Cancelled by seller"
	if userRole == "admin" {
		reason = "Cancelled by admin"
	}
	
	userIDValue4 := userID.(uint64)
	history := &models.AuctionStatusHistory{
		AuctionID:  auctionID,
		FromStatus: oldStatus,
		ToStatus:   auction.StatusCode,
		Reason:     reason,
		OperatorID: &userIDValue4,
	}
	h.DB.Create(history)

	// WebSocket 廣播拍賣取消事件
	if h.WSHandler != nil {
		stateData := map[string]interface{}{
			"status_code": auction.StatusCode,
			"reason":      reason,
		}
		
		h.WSHandler.Hub.BroadcastToAuction(
			auctionID,
			websocket.MessageTypeClosed,
			stateData,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"status_code": auction.StatusCode,
	})
}