package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"auction_service/internal/config"
	"auction_service/internal/database"
	"auction_service/internal/handlers"
	"auction_service/internal/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	// 載入配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 初始化 logger
	logger, err := logger.New(cfg)
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	logger.Info("Starting auction service",
		zap.String("service", cfg.AppName),
		zap.String("env", cfg.AppEnv),
		zap.String("port", cfg.AppPort),
	)

	logger.Debug("Configuration loaded",
		zap.String("app_name", cfg.AppName),
		zap.String("app_env", cfg.AppEnv),
		zap.String("app_port", cfg.AppPort),
		zap.String("db_host", cfg.DBHost),
		zap.String("db_port", cfg.DBPort),
		zap.String("db_name", cfg.DBName),
		zap.String("redis_host", cfg.RedisHost),
		zap.String("jwt_issuer", cfg.JWTIssuer),
		zap.String("cors_origins", cfg.CORSAllowedOrigins),
	)

	// 連接資料庫（可選，用於 Cloud Run 部署）
	var db interface{} // Use interface{} to allow nil
	if cfg.AppEnv != "production" || cfg.DBHost != "localhost" {
		logger.Info("Connecting to database",
			zap.String("host", cfg.DBHost),
			zap.String("port", cfg.DBPort),
			zap.String("database", cfg.DBName),
		)
		
		if dbConn, err := database.Connect(cfg); err != nil {
			logger.Warn("Failed to connect to database, continuing without DB", 
				zap.String("host", cfg.DBHost),
				zap.String("port", cfg.DBPort),
				zap.String("database", cfg.DBName),
				zap.Error(err),
			)
		} else {
			db = dbConn
			logger.Info("Database connection established successfully",
				zap.String("host", cfg.DBHost),
				zap.String("database", cfg.DBName),
			)
		}
	} else {
		logger.Info("Skipping database connection in production with localhost")
	}

	// 自動遷移（開發環境）- 跳過，使用手動遷移
	if cfg.AppEnv == "development" && db != nil {
		logger.Info("Skipping auto migrations - using manual migrations for better control")
		
		// Verify database connectivity
		if gormDB, ok := db.(*gorm.DB); ok {
			if sqlDB, err := gormDB.DB(); err == nil {
				if err := sqlDB.Ping(); err != nil {
					logger.Warn("Database ping failed", zap.Error(err))
				} else {
					logger.Info("Database connectivity verified")
				}
			}
		}
	}

	// 連接 Redis（可選）
	var redisClient *redis.Client
	if cfg.RedisHost != "" {
		logger.Info("Connecting to Redis",
			zap.String("host", cfg.RedisHost),
			zap.Int("db", cfg.RedisDB),
		)
		
		redisClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisHost,
			DB:   cfg.RedisDB,
		})
		
		// 測試連接
		ctx := context.Background()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			logger.Warn("Failed to connect to Redis", 
				zap.String("host", cfg.RedisHost),
				zap.Error(err),
			)
			redisClient = nil
		} else {
			logger.Info("Redis connection established successfully", 
				zap.String("host", cfg.RedisHost),
				zap.Int("db", cfg.RedisDB),
			)
		}
	} else {
		logger.Info("Redis not configured, skipping connection")
	}

	// 初始化路由
	logger.Info("Initializing HTTP routes and middleware")
	
	// Cast db back to *gorm.DB or nil
	var gormDB *gorm.DB
	if db != nil {
		if castedDB, ok := db.(*gorm.DB); ok {
			gormDB = castedDB
		}
	}
	
	router := handlers.NewRouter(cfg, logger, gormDB, redisClient)
	logger.Info("HTTP routes initialized successfully")

	// 啟動服務器
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.AppPort),
		Handler: router,
	}

	logger.Info("Server starting", 
		zap.String("service", cfg.AppName),
		zap.String("env", cfg.AppEnv),
		zap.String("address", server.Addr),
		zap.Bool("database_connected", gormDB != nil),
		zap.Bool("redis_connected", redisClient != nil),
	)
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Failed to start server", 
			zap.String("address", server.Addr),
			zap.Error(err),
		)
	}

	logger.Info("Server shutdown completed")
}