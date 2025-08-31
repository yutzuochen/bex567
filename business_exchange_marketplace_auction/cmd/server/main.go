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

	// 連接資料庫
	logger.Info("Connecting to database",
		zap.String("host", cfg.DBHost),
		zap.String("port", cfg.DBPort),
		zap.String("database", cfg.DBName),
	)
	
	db, err := database.Connect(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to database", 
			zap.String("host", cfg.DBHost),
			zap.String("port", cfg.DBPort),
			zap.String("database", cfg.DBName),
			zap.Error(err),
		)
	}

	logger.Info("Database connection established successfully",
		zap.String("host", cfg.DBHost),
		zap.String("database", cfg.DBName),
	)

	// 自動遷移（開發環境）- 跳過，使用手動遷移
	if cfg.AppEnv == "development" {
		logger.Info("Skipping auto migrations - using manual migrations for better control")
		
		// Verify database connectivity
		sqlDB, err := db.DB()
		if err == nil {
			if err := sqlDB.Ping(); err != nil {
				logger.Warn("Database ping failed", zap.Error(err))
			} else {
				logger.Info("Database connectivity verified")
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
	router := handlers.NewRouter(cfg, logger, db, redisClient)
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
		zap.Bool("database_connected", db != nil),
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