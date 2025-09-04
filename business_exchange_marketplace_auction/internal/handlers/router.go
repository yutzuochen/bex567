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

	// 初始化 WebSocket 處理器（僅在有數據庫時）
	var wsHandler *websocket.Handler
	if db != nil {
		wsHandler = websocket.NewHandler(db, redisClient, logger, cfg)
		wsHandler.Start(context.Background())
	} else {
		logger.Info("Skipping WebSocket handler initialization - no database connection")
	}

	// API v1 路由
	api := r.Group("/api/v1")
	
	if db != nil {
		// 完整功能（有數據庫時）
		auctionHandler := &AuctionHandler{DB: db, Logger: logger, WSHandler: wsHandler}
		bidHandler := &BidHandler{DB: db, Logger: logger, WSHandler: wsHandler}
		blacklistHandler := &BlacklistHandler{DB: db, Logger: logger}
		authHandler := &AuthHandler{DB: db, Logger: logger, JWTSecret: cfg.JWTSecret}

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
	} else {
		// 模擬端點（無數據庫時）- 提供示例英式拍賣資料
		logger.Info("Registering mock API endpoints - no database connection")
		
		// 模擬認證端點（無數據庫時）
		api.GET("/auth/me", func(c *gin.Context) {
			// 返回模擬用戶信息，表示用戶已認證
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"id":    1,
					"email": "demo@example.com",
					"name":  "Demo User",
					"role":  "user",
				},
			})
		})

		// 公開端點（無需認證）- 返回模擬拍賣資料
		api.GET("/auctions", func(c *gin.Context) {
			// 模擬兩個英式拍賣
			mockAuctions := []map[string]interface{}{
				{
					"auction_id":     1,
					"title":         "高端商業咖啡廳轉讓 - 台北信義區黃金地段",
					"description":   "位於台北信義區精華地段的高端商業咖啡廳，擁有完整設備、穩定客源及優質裝潢。適合有餐飲經驗的投資者接手營運。",
					"auction_type":  "english",
					"status":        "active",
					"start_at":      "2025-09-01T10:00:00Z",
					"end_at":        "2025-09-10T18:00:00Z",
					"allowed_min_bid": 800000,
					"allowed_max_bid": 1500000,
					"current_price": 950000,
					"bid_count":     12,
					"seller_id":     1,
					"seller_name":   "王先生",
					"location":      "台北市信義區",
					"category":      "餐飲業",
					"reserve_met":   true,
					"created_at":    "2025-09-01T09:00:00Z",
					"updated_at":    "2025-09-02T15:30:00Z",
				},
				{
					"auction_id":     2,
					"title":         "科技新創公司股權轉讓 - AI人工智慧解決方案",
					"description":   "專精於AI人工智慧解決方案的新創公司，擁有多項專利技術、優秀團隊及穩定營收。尋求策略投資夥伴共同發展。",
					"auction_type":  "english",
					"status":        "active", 
					"start_at":      "2025-09-02T14:00:00Z",
					"end_at":        "2025-09-15T20:00:00Z",
					"allowed_min_bid": 2000000,
					"allowed_max_bid": 5000000,
					"current_price": 2350000,
					"bid_count":     8,
					"seller_id":     2,
					"seller_name":   "李小姐",
					"location":      "新北市板橋區",
					"category":      "科技業",
					"reserve_met":   false,
					"created_at":    "2025-09-02T13:00:00Z", 
					"updated_at":    "2025-09-02T16:45:00Z",
				},
			}
			
			c.JSON(http.StatusOK, gin.H{
				"auctions": mockAuctions,
				"total":    2,
				"page":     1,
				"per_page": 10,
			})
		})
		
		api.GET("/auctions/:id", func(c *gin.Context) {
			auctionID := c.Param("id")
			
			// 根據 ID 返回對應的模擬資料
			if auctionID == "1" {
				c.JSON(http.StatusOK, map[string]interface{}{
					"auction_id":     1,
					"title":         "高端商業咖啡廳轉讓 - 台北信義區黃金地段",
					"description":   "位於台北信義區精華地段的高端商業咖啡廳，擁有完整設備、穩定客源及優質裝潢。適合有餐飲經驗的投資者接手營運。詳細包含：\\n- 營業面積：80坪\\n- 座位數：50席\\n- 月營業額：約150萬元\\n- 每月租金：12萬元\\n- 設備價值：約200萬元\\n- 員工：8名（可續聘）",
					"auction_type":  "english",
					"status":        "active",
					"start_at":      "2025-09-01T10:00:00Z",
					"end_at":        "2025-09-10T18:00:00Z",
					"allowed_min_bid": 800000,
					"allowed_max_bid": 1500000,
					"current_price": 950000,
					"bid_count":     12,
					"seller_id":     1,
					"seller_name":   "王先生",
					"location":      "台北市信義區",
					"category":      "餐飲業",
					"reserve_met":   true,
					"created_at":    "2025-09-01T09:00:00Z",
					"updated_at":    "2025-09-02T15:30:00Z",
				})
			} else if auctionID == "2" {
				c.JSON(http.StatusOK, map[string]interface{}{
					"auction_id":     2,
					"title":         "科技新創公司股權轉讓 - AI人工智慧解決方案",
					"description":   "專精於AI人工智慧解決方案的新創公司，擁有多項專利技術、優秀團隊及穩定營收。尋求策略投資夥伴共同發展。詳細資訊：\\n- 成立時間：2022年\\n- 員工人數：25名\\n- 年營收：約1200萬元\\n- 專利技術：3項\\n- 主要客戶：5家上市公司\\n- 轉讓股權：30%",
					"auction_type":  "english",
					"status":        "active",
					"start_at":      "2025-09-02T14:00:00Z", 
					"end_at":        "2025-09-15T20:00:00Z",
					"allowed_min_bid": 2000000,
					"allowed_max_bid": 5000000,
					"current_price": 2350000,
					"bid_count":     8,
					"seller_id":     2,
					"seller_name":   "李小姐",
					"location":      "新北市板橋區", 
					"category":      "科技業",
					"reserve_met":   false,
					"created_at":    "2025-09-02T13:00:00Z",
					"updated_at":    "2025-09-02T16:45:00Z",
				})
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "拍賣不存在"})
			}
		})
		
		api.GET("/auctions/:id/stats/histogram", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"histogram": []map[string]interface{}{
					{"price_range": "800000-900000", "bid_count": 3},
					{"price_range": "900000-1000000", "bid_count": 5},
					{"price_range": "1000000-1100000", "bid_count": 4},
				},
			})
		})
	}

	// WebSocket 路由（僅在有 WebSocket 處理器時）
	if wsHandler != nil {
		websocket.SetupRoutes(r, wsHandler)
	} else {
		logger.Info("Skipping WebSocket routes setup - no WebSocket handler")
	}

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