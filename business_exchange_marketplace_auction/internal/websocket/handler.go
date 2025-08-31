package websocket

import (
	"context"
	"net/http"
	"strconv"

	"auction_service/internal/config"
	"auction_service/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Handler WebSocket 處理器
type Handler struct {
	Hub    *Hub
	Logger *zap.Logger
	Config *config.Config
}

// NewHandler 創建新的 WebSocket 處理器
func NewHandler(db *gorm.DB, redis *redis.Client, logger *zap.Logger, config *config.Config) *Handler {
	hub := NewHub(db, redis, logger, config)
	
	return &Handler{
		Hub:    hub,
		Logger: logger,
		Config: config,
	}
}

// Start 啟動 WebSocket Hub
func (h *Handler) Start(ctx context.Context) {
	go h.Hub.Run(ctx)
}

// HandleConnection 處理 WebSocket 連接
func (h *Handler) HandleConnection(c *gin.Context) {
	requestID := c.GetString("request_id")
	clientIP := c.ClientIP()
	
	h.Logger.Info("WebSocket connection attempt",
		zap.String("request_id", requestID),
		zap.String("client_ip", clientIP),
		zap.String("user_agent", c.Request.UserAgent()),
	)

	// 檢查認證
	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.Logger.Warn("WebSocket connection rejected - no authentication",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
		)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}
	
	userIDValue, ok := userID.(uint64)
	if !ok {
		h.Logger.Warn("WebSocket connection rejected - invalid user token type",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Any("user_id_value", userID),
		)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid user token",
		})
		return
	}
	
	h.Logger.Debug("WebSocket user authenticated",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
	)
	
	// 取得拍賣 ID
	auctionIDStr := c.Param("auction_id")
	h.Logger.Debug("Parsing auction ID",
		zap.String("request_id", requestID),
		zap.String("auction_id_str", auctionIDStr),
	)
	
	auctionID, err := strconv.ParseUint(auctionIDStr, 10, 64)
	if err != nil {
		h.Logger.Warn("WebSocket connection rejected - invalid auction ID",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", userIDValue),
			zap.String("auction_id_str", auctionIDStr),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid auction ID",
		})
		return
	}
	
	// 檢查拍賣是否存在且有效
	h.Logger.Debug("Validating auction",
		zap.String("request_id", requestID),
		zap.Uint64("auction_id", auctionID),
	)
	
	if !h.isValidAuction(auctionID) {
		h.Logger.Warn("WebSocket connection rejected - auction not found or inactive",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", userIDValue),
			zap.Uint64("auction_id", auctionID),
		)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Auction not found or inactive",
		})
		return
	}
	
	h.Logger.Info("Upgrading to WebSocket connection",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("auction_id", auctionID),
	)
	
	// 升級連接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.Logger.Error("Failed to upgrade WebSocket connection", 
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", userIDValue),
			zap.Uint64("auction_id", auctionID),
			zap.Error(err),
		)
		return
	}
	
	h.Logger.Info("WebSocket connection established successfully",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("auction_id", auctionID),
		zap.String("remote_addr", conn.RemoteAddr().String()),
	)
	
	// 創建新連接
	wsConn := NewConnection(h.Hub, conn, auctionID, userIDValue, h.Logger)
	
	h.Logger.Debug("Starting WebSocket connection handler",
		zap.String("request_id", requestID),
		zap.String("connection_id", wsConn.ID),
		zap.Uint64("user_id", userIDValue),
		zap.Uint64("auction_id", auctionID),
	)
	
	// 啟動連接處理
	wsConn.Start()
}

// GetStats 取得統計資訊
func (h *Handler) GetStats(c *gin.Context) {
	stats := h.Hub.GetStats()
	
	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}

// BroadcastToAuction 向拍賣房間廣播訊息 (供其他服務調用)
func (h *Handler) BroadcastToAuction(auctionID uint64, msgType string, data interface{}) {
	h.Hub.BroadcastToAuction(auctionID, msgType, data)
}

// BroadcastToUser 向特定用戶廣播訊息 (供其他服務調用)
func (h *Handler) BroadcastToUser(auctionID, userID uint64, msgType string, data interface{}) {
	h.Hub.BroadcastToUser(auctionID, userID, msgType, data)
}

// isValidAuction 檢查拍賣是否有效
func (h *Handler) isValidAuction(auctionID uint64) bool {
	var count int64
	h.Hub.DB.Table("auctions").
		Where("auction_id = ? AND status_code IN ('active', 'extended')", auctionID).
		Count(&count)
	
	return count > 0
}

// SetupRoutes 設定 WebSocket 路由
func SetupRoutes(router *gin.Engine, handler *Handler) {
	ws := router.Group("/ws")
	{
		// WebSocket 連接端點 - 需要認證
		ws.GET("/auctions/:auction_id", middleware.JWT(handler.Config), handler.HandleConnection)
		
		// 統計資訊端點
		ws.GET("/stats", handler.GetStats)
	}
}