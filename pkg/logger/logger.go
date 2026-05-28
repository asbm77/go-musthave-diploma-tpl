package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger глобальный экземпляр логгера
var Logger *zap.SugaredLogger

// Initialize инициализирует логгер с указанным уровнем
func Initialize(level string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		return err
	}

	Logger = logger.Sugar()
	return nil
}

// Sync синхронизирует логгер
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}
