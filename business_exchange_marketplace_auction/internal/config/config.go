package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// 應用程式設定
	AppEnv  string
	AppPort string
	AppName string

	// JWT 設定
	JWTSecret string
	JWTIssuer string

	// 資料庫設定
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Redis 設定
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// 通知設定
	EmailSMTPHost string
	EmailSMTPPort string
	EmailUsername string
	EmailPassword string

	// 降級設定
	DegradedCPUThreshold        int
	DegradedRedisErrorThreshold int
	DegradedDBLatencyThreshold  int

	// CORS 設定
	CORSAllowedOrigins string
	CORSAllowedMethods string
	CORSAllowedHeaders string
}

func Load() (*Config, error) {
	// 嘗試載入 .env 檔案（開發環境）
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:  getEnv("APP_ENV", "development"),
		AppPort: getEnv("APP_PORT", "8081"),
		AppName: getEnv("APP_NAME", "auction_service"),

		JWTSecret: getEnv("JWT_SECRET", "default-jwt-secret-change-in-production"),
		JWTIssuer: getEnv("JWT_ISSUER", "auction-service"),

		DBHost:     getEnv("DB_HOST", "127.0.0.1"), // TODO:need to modify for deploying to Google Cloud SQL
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "app"),
		DBPassword: getEnv("DB_PASSWORD", "app_password"),
		DBName:     getEnv("DB_NAME", "business_exchange"),

		RedisHost:     getEnv("REDIS_HOST", "127.0.0.1"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		EmailSMTPHost: getEnv("EMAIL_SMTP_HOST", "smtp.gmail.com"),
		EmailSMTPPort: getEnv("EMAIL_SMTP_PORT", "587"),
		EmailUsername: getEnv("EMAIL_USERNAME", ""),
		EmailPassword: getEnv("EMAIL_PASSWORD", ""),

		DegradedCPUThreshold:        getEnvAsInt("DEGRADED_CPU_THRESHOLD", 80),
		DegradedRedisErrorThreshold: getEnvAsInt("DEGRADED_REDIS_ERROR_THRESHOLD", 5),
		DegradedDBLatencyThreshold:  getEnvAsInt("DEGRADED_DB_LATENCY_THRESHOLD", 500),

		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://127.0.0.1:3000"),
		CORSAllowedMethods: getEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS"),
		CORSAllowedHeaders: getEnv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Idempotency-Key"),
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.JWTSecret == "default-jwt-secret-change-in-production" && c.AppEnv == "production" {
		return fmt.Errorf("JWT_SECRET must be set in production")
	}

	if c.DBPassword == "" {
		return fmt.Errorf("DB_PASSWORD is required")
	}

	return nil
}

func (c *Config) GetDBDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
