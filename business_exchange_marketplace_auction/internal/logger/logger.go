package logger

import (
	"auction_service/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg *config.Config) (*zap.Logger, error) {
	var logConfig zap.Config

	if cfg.AppEnv == "production" {
		logConfig = zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	} else {
		logConfig = zap.NewDevelopmentConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		logConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := logConfig.Build()
	if err != nil {
		return nil, err
	}

	// 增加一些全域字段
	logger = logger.With(
		zap.String("service", cfg.AppName),
		zap.String("env", cfg.AppEnv),
	)

	return logger, nil
}