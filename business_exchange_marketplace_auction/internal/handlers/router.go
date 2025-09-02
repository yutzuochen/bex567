package handlers

import (
	"context"
	"net/http"
	"time"

	"auction_service/internal/config"
	"auction_service/internal/middleware"
	"auction_service/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewRouter(cfg *config.Config, logger *zap.Logger, db *gorm.DB, redisClient *redis.Client) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// 全域中間件
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS(cfg))
	r.Use(loggerMiddleware(logger))
	r.Use(requestLogger(logger))

	// 健康檢查端點
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"service":    cfg.AppName,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": c.GetString(middleware.RequestIDKey),
		})
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 狀態端點
	r.GET("/api/v1/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"healthy":        true,
			"degraded_level": 0,
			"now":           time.Now().UTC().Format(time.RFC3339),
		})
	})

	// 初始化 WebSocket 處理器
	wsHandler := websocket.NewHandler(db, redisClient, logger, cfg)
	wsHandler.Start(context.Background())

	// 初始化處理器
	auctionHandler := &AuctionHandler{DB: db, Logger: logger, WSHandler: wsHandler}
	bidHandler := &BidHandler{DB: db, Logger: logger, WSHandler: wsHandler}
	blacklistHandler := &BlacklistHandler{DB: db, Logger: logger}
	authHandler := &AuthHandler{DB: db, Logger: logger, JWTSecret: cfg.JWTSecret}

	// API v1 路由
	api := r.Group("/api/v1")
	{
		// 公開端點（無需認證）
		api.GET("/auctions", auctionHandler.ListAuctions)
		api.GET("/auctions/:id", auctionHandler.GetAuction)
		api.GET("/auctions/:id/stats/histogram", bidHandler.GetAuctionHistogram)

		// 需要認證的端點
		auth := api.Group("")
		auth.Use(middleware.JWT(cfg))
		{
			// 認證相關
			auth.GET("/auth/ws-token", authHandler.GetWebSocketToken)

			// 拍賣管理（賣家）
			auth.POST("/auctions", auctionHandler.CreateAuction)
			auth.POST("/auctions/:id/activate", auctionHandler.ActivateAuction)
			auth.POST("/auctions/:id/cancel", auctionHandler.CancelAuction)

			// 出價（買家）
			auth.POST("/auctions/:id/bids", bidHandler.PlaceBid)
			auth.POST("/auctions/:id/buy-now", bidHandler.BuyItNow) // 英式拍賣直購
			auth.GET("/auctions/:id/my-bids", bidHandler.GetMyBids)
			auth.GET("/auctions/:id/results", bidHandler.GetAuctionResults)

			// 管理員端點
			admin := auth.Group("/admin")
			admin.Use(middleware.RequireRole("admin"))
			{
				// 黑名單管理
				admin.GET("/blacklist", blacklistHandler.ListBlacklist)
				admin.POST("/blacklist", blacklistHandler.AddBlacklist)
				admin.DELETE("/blacklist/:user_id", blacklistHandler.RemoveBlacklist)

				// 管理員拍賣操作
				admin.POST("/auctions/:id/finalize", func(c *gin.Context) {
					c.JSON(http.StatusNotImplemented, gin.H{
						"message": "Not implemented yet",
					})
				})

				admin.POST("/auctions/:id/recompute-histogram", func(c *gin.Context) {
					c.JSON(http.StatusNotImplemented, gin.H{
						"message": "Not implemented yet",
					})
				})

				admin.GET("/auctions/:id/bids", func(c *gin.Context) {
					c.JSON(http.StatusNotImplemented, gin.H{
						"message": "Not implemented yet",
					})
				})
			}
		}
	}

	// WebSocket 路由
	websocket.SetupRoutes(r, wsHandler)

	return r
}

// loggerMiddleware adds the logger to the Gin context for use by other middleware
func loggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("logger", logger)
		c.Next()
	}
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		requestID := c.GetString(middleware.RequestIDKey)
		if requestID == "" {
			requestID = "unknown"
		}

		logger.Info("request",
			zap.String("request_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
		)
	}
}