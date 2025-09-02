package websocket

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"auction_service/internal/config"
	"auction_service/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

	// Enhanced logging for debugging
	log.Printf("[WS] HandleConnection called - Method=%s, Path=%s, URL=%s",
		c.Request.Method, c.Request.URL.Path, c.Request.URL.String())
	log.Printf("[WS] Request Headers: %v", c.Request.Header)
	log.Printf("[WS] Cookies: %v", c.Request.Cookies())

	h.Logger.Info("WebSocket connection attempt",
		zap.String("request_id", requestID),
		zap.String("client_ip", clientIP),
		zap.String("user_agent", c.Request.UserAgent()),
		zap.String("origin", c.GetHeader("Origin")),
		zap.String("upgrade", c.GetHeader("Upgrade")),
		zap.String("connection", c.GetHeader("Connection")),
		zap.String("sec_websocket_key", c.GetHeader("Sec-WebSocket-Key")),
		zap.String("sec_websocket_version", c.GetHeader("Sec-WebSocket-Version")),
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

	log.Printf("[WS] About to upgrade connection...")

	// 升級連接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade FAILED: %v", err)
		h.Logger.Error("Failed to upgrade WebSocket connection",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", userIDValue),
			zap.Uint64("auction_id", auctionID),
			zap.Error(err),
		)
		return
	}

	log.Printf("[WS] Upgrade SUCCESS!")

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

// validateJWT 簡單JWT驗證（用於測試）
func (h *Handler) validateJWT(tokenString string) (*middleware.JWTClaims, bool) {
	if tokenString == "" {
		return nil, false
	}

	// Parse the token
	token, err := jwt.ParseWithClaims(tokenString, &middleware.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.Config.JWTSecret), nil
	})

	if err != nil {
		log.Printf("[WS] JWT validation error: %v", err)
		return nil, false
	}

	if claims, ok := token.Claims.(*middleware.JWTClaims); ok && token.Valid {
		return claims, true
	}

	return nil, false
}

// HandleTestConnection 測試用的簡化WebSocket處理器
func (h *Handler) HandleTestConnection(c *gin.Context) {
	log.Printf("[WS] HandleTestConnection called - Method=%s, Path=%s",
		c.Request.Method, c.Request.URL.Path)
	log.Printf("[WS] Query params: %v", c.Request.URL.Query())
	log.Printf("[WS] All Headers: %v", c.Request.Header)

	// 1) 從查詢參數驗證JWT
	token := c.Query("token")
	if token == "" {
		log.Printf("[WS] reject: missing token in query")
		c.Writer.WriteHeader(http.StatusUnauthorized)
		c.Writer.Write([]byte("missing token"))
		return
	}

	claims, valid := h.validateJWT(token)
	if !valid {
		log.Printf("[WS] reject: invalid token")
		c.Writer.WriteHeader(http.StatusUnauthorized)
		c.Writer.Write([]byte("invalid token"))
		return
	}

	log.Printf("[WS] JWT valid for user %d", claims.UserID)

	// 2) 取得拍賣ID
	auctionIDStr := c.Param("auction_id")
	auctionID, err := strconv.ParseUint(auctionIDStr, 10, 64)
	if err != nil {
		log.Printf("[WS] reject: invalid auction_id=%s", auctionIDStr)
		c.Writer.WriteHeader(http.StatusBadRequest)
		c.Writer.Write([]byte("invalid auction_id"))
		return
	}

	log.Printf("[WS] Auction ID: %d", auctionID)

	// 3) 嘗試升級連接
	log.Printf("[WS] About to upgrade to WebSocket...")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade FAILED: %v", err)
		return
	}

	log.Printf("[WS] Upgrade SUCCESS! Starting echo loop...")
	defer conn.Close()

	// 4) 發送歡迎訊息
	welcomeMsg := map[string]interface{}{
		"type": "welcome",
		"data": map[string]interface{}{
			"user_id":    claims.UserID,
			"auction_id": auctionID,
			"message":    "WebSocket connection established successfully!",
		},
	}

	if err := conn.WriteJSON(welcomeMsg); err != nil {
		log.Printf("[WS] Failed to send welcome message: %v", err)
		return
	}

	// 5) Echo loop for testing
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WS] Read error: %v", err)
			break
		}

		log.Printf("[WS] Received message type=%d, content=%s", messageType, string(message))

		// Echo the message back
		if err := conn.WriteMessage(messageType, message); err != nil {
			log.Printf("[WS] Write error: %v", err)
			break
		}

		log.Printf("[WS] Echoed message back to client")
	}

	log.Printf("[WS] Connection closed")
}

// SetupRoutes 設定 WebSocket 路由
func SetupRoutes(router *gin.Engine, handler *Handler) {
	ws := router.Group("/ws")
	{
		// WebSocket 連接端點 - 需要認證
		ws.GET("/auctions/:auction_id", middleware.JWT(handler.Config), handler.HandleConnection)

		// 測試用的 WebSocket 端點 - JWT 從查詢參數取得
		ws.GET("/test/:auction_id", handler.HandleTestConnection)

		// 統計資訊端點
		ws.GET("/stats", handler.GetStats)
	}
}
