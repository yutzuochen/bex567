package middleware

import (
	"net/http"
	"strings"

	"auction_service/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

const (
	UserIDKey   = "user_id"
	UserRoleKey = "user_role"
)

type JWTClaims struct {
	UserID uint   `json:"uid"`        // Match main backend format exactly
	Email  string `json:"email"`
	Role   string `json:"role,omitempty"` // Optional field
	jwt.RegisteredClaims
}

func JWT(cfg *config.Config) gin.HandlerFunc {
	// Get logger from gin context or create a no-op logger
	logger := zap.NewNop()
	
	return func(c *gin.Context) {
		// Try to get logger from context
		if ctxLogger, exists := c.Get("logger"); exists {
			if l, ok := ctxLogger.(*zap.Logger); ok {
				logger = l
			}
		}
		
		requestID := c.GetString("request_id")
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		
		logger.Debug("JWT middleware processing request",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("Missing Authorization header",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.String("user_agent", userAgent),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Authorization header required",
			}})
			c.Abort()
			return
		}

		logger.Debug("Authorization header present",
			zap.String("request_id", requestID),
			zap.String("auth_header_prefix", authHeader[:min(20, len(authHeader))]), // Only log first 20 chars for security
		)

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			logger.Warn("Invalid authorization header format",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Int("token_parts_count", len(tokenParts)),
				zap.String("first_part", func() string {
					if len(tokenParts) > 0 { return tokenParts[0] }
					return ""
				}()),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Invalid authorization header format",
			}})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]
		logger.Debug("Parsing JWT token",
			zap.String("request_id", requestID),
			zap.Int("token_length", len(tokenString)),
		)
		
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			logger.Debug("JWT signing method validation",
				zap.String("request_id", requestID),
				zap.String("signing_method", token.Method.Alg()),
			)
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			logger.Warn("Invalid JWT token",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Error(err),
				zap.Bool("token_valid", token != nil && token.Valid),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Invalid token",
			}})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*JWTClaims)
		if !ok {
			logger.Warn("Invalid JWT claims type",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Invalid token claims",
			}})
			c.Abort()
			return
		}

		logger.Info("JWT authentication successful",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", uint64(claims.UserID)), // Convert uint to uint64
			zap.String("user_email", claims.Email),
			zap.String("user_role", claims.Role),
			zap.Time("token_expires_at", claims.ExpiresAt.Time),
		)

		c.Set(UserIDKey, uint64(claims.UserID)) // Store as uint64 for consistency
		c.Set(UserRoleKey, claims.Role)
		
		c.Next()
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger := zap.NewNop()
		if ctxLogger, exists := c.Get("logger"); exists {
			if l, ok := ctxLogger.(*zap.Logger); ok {
				logger = l
			}
		}
		
		requestID := c.GetString("request_id")
		clientIP := c.ClientIP()
		userID, _ := c.Get(UserIDKey)
		
		logger.Debug("Role authorization check",
			zap.String("request_id", requestID),
			zap.String("required_role", role),
			zap.Any("user_id", userID),
		)

		userRole, exists := c.Get(UserRoleKey)
		if !exists {
			logger.Warn("Role information not available in context",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Any("user_id", userID),
			)
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "forbidden",
				"message": "Role information not available",
			}})
			c.Abort()
			return
		}

		if userRole != role {
			logger.Warn("Insufficient permissions for role access",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Any("user_id", userID),
				zap.String("user_role", userRole.(string)),
				zap.String("required_role", role),
			)
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "forbidden",
				"message": "Insufficient permissions",
			}})
			c.Abort()
			return
		}

		logger.Info("Role authorization successful",
			zap.String("request_id", requestID),
			zap.Any("user_id", userID),
			zap.String("user_role", userRole.(string)),
			zap.String("required_role", role),
		)

		c.Next()
	}
}