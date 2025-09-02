package handlers

import (
	"net/http"
	"time"

	"auction_service/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB        *gorm.DB
	Logger    *zap.Logger
	JWTSecret string
}

// WebSocketTokenResponse WebSocket token 響應
type WebSocketTokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // 秒數
}

// GetWebSocketToken 獲取 WebSocket 連接用的臨時 token
// GET /api/v1/auth/ws-token
func (h *AuthHandler) GetWebSocketToken(c *gin.Context) {
	requestID := c.GetString("request_id")
	clientIP := c.ClientIP()
	
	h.Logger.Info("Getting WebSocket token",
		zap.String("request_id", requestID),
		zap.String("client_ip", clientIP),
	)

	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.Logger.Warn("WebSocket token request without authentication",
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
	h.Logger.Info("Creating WebSocket token for user",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
	)

	// 創建一個短期的 JWT token (5 分鐘有效)
	expiresIn := 300 // 5 minutes
	claims := jwt.MapClaims{
		"uid": userIDValue,
		"iss": "auction_service",
		"exp": time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
		"iat": time.Now().Unix(),
		"purpose": "websocket", // 標記這是 WebSocket 專用 token
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.JWTSecret))
	if err != nil {
		h.Logger.Error("Failed to sign WebSocket token",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userIDValue),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to create WebSocket token",
		}})
		return
	}

	h.Logger.Info("WebSocket token created successfully",
		zap.String("request_id", requestID),
		zap.Uint64("user_id", userIDValue),
		zap.Int("expires_in", expiresIn),
	)

	c.JSON(http.StatusOK, gin.H{
		"data": WebSocketTokenResponse{
			Token:     tokenString,
			ExpiresIn: expiresIn,
		},
	})
}