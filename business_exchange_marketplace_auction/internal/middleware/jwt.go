package middleware

import (
	"fmt"
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

// JWT claims are now handled using jwt.MapClaims for consistency with main backend

func JWT(cfg *config.Config) gin.HandlerFunc {
	// Get logger from gin context or create a no-op logger
	logger := zap.NewNop()

	logger.Debug("============= Welcome to JWT =============")
	logger.Info("JWT middleware initialized",
		zap.String("jwt_secret_length", fmt.Sprintf("%d", len(cfg.JWTSecret))),
		zap.String("jwt_issuer", cfg.JWTIssuer),
		zap.String("app_env", cfg.AppEnv),
	)
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

		var tokenString string

		// First, try to get token from cookie (preferred method)
		if cookie, err := c.Cookie("authToken"); err == nil && cookie != "" {
			tokenString = cookie
			logger.Debug("Token found in cookie",
				zap.String("request_id", requestID),
			)
		} else {
			// Try to get token from Authorization header
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
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
							if len(tokenParts) > 0 {
								return tokenParts[0]
							}
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
				tokenString = tokenParts[1]
			} else {
				// Try to get token from query parameter (for WebSocket connections)
				tokenString = c.Query("token")
				if tokenString == "" {
					logger.Warn("Missing Authentication: no cookie, Authorization header, or token query parameter",
						zap.String("request_id", requestID),
						zap.String("client_ip", clientIP),
						zap.String("user_agent", userAgent),
					)
					c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
						"code":    "unauthorized",
						"message": "Authentication required: no token found in cookie, Authorization header, or query parameter",
					}})
					c.Abort()
					return
				}
				logger.Debug("Token found in query parameter",
					zap.String("request_id", requestID),
					zap.Int("token_length", len(tokenString)),
				)
			}
		}
		logger.Debug("Parsing JWT token",
			zap.String("request_id", requestID),
			zap.Int("token_length", len(tokenString)),
		)

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			logger.Debug("JWT signing method validation",
				zap.String("request_id", requestID),
				zap.String("signing_method", token.Method.Alg()),
			)
			// Ensure the token method is HMAC256
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				logger.Error("JWT: Invalid signing method",
					zap.String("request_id", requestID),
					zap.String("client_ip", clientIP),
					zap.String("method", fmt.Sprintf("%T", token.Method)),
				)
				return nil, jwt.ErrSignatureInvalid
			}
			logger.Debug("JWT: Token signing method validated",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
			)
			return []byte(cfg.JWTSecret), nil
		})

		logger.Debug("JWT parsing result",
			zap.String("request_id", requestID),
			zap.Error(err),
			zap.Bool("token_valid", token != nil && token.Valid),
		)

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

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			logger.Warn("Invalid JWT claims type",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Any("claims_type", fmt.Sprintf("%T", token.Claims)),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Invalid token claims",
			}})
			c.Abort()
			return
		}

		// Validate issuer
		if issuer, exists := claims["iss"]; !exists || issuer != cfg.JWTIssuer {
			logger.Warn("Invalid or missing token issuer",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
				zap.Any("found_issuer", issuer),
				zap.String("expected_issuer", cfg.JWTIssuer),
				zap.Bool("issuer_exists", exists),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Invalid token issuer",
			}})
			c.Abort()
			return
		}

		// Extract user ID
		var userID uint64
		if uid, exists := claims["uid"]; exists {
			if userIDFloat, ok := uid.(float64); ok {
				userID = uint64(userIDFloat)
			} else {
				logger.Warn("Invalid user ID type in token",
					zap.String("request_id", requestID),
					zap.String("client_ip", clientIP),
					zap.Any("user_id", uid),
				)
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
					"code":    "unauthorized",
					"message": "Invalid token user ID",
				}})
				c.Abort()
				return
			}
		} else {
			logger.Warn("Missing user ID in token",
				zap.String("request_id", requestID),
				zap.String("client_ip", clientIP),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code":    "unauthorized",
				"message": "Missing user ID in token",
			}})
			c.Abort()
			return
		}

		// Extract email and role (optional)
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)

		logger.Debug("JWT claims parsed successfully",
			zap.String("request_id", requestID),
			zap.Uint64("user_id", userID),
			zap.String("email", email),
			zap.String("role", role),
			zap.String("issuer", cfg.JWTIssuer),
		)

		logger.Info("JWT authentication successful",
			zap.String("request_id", requestID),
			zap.String("client_ip", clientIP),
			zap.Uint64("user_id", userID),
			zap.String("user_email", email),
			zap.String("user_role", role),
		)

		c.Set(UserIDKey, userID)
		c.Set(UserRoleKey, role)

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
