package log

import (
	"os"

	"go.uber.org/zap"
)

var logger *zap.Logger

func Init() {
	config := zap.NewProductionConfig()

	// Set the log format
	format := os.Getenv("LOG_FORMAT")
	if format == "test" {
		config.Encoding = "console"
	} else {
		config.Encoding = "json"
	}

	// Set the log level
	level := os.Getenv("LOG_LEVEL")
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Build the logger
	var err error
	logger, err = config.Build(zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}

	// Ensure we flush any buffered log entries
	defer logger.Sync()
}

func Info(message string, fields ...zap.Field) {
	logger.Info(message, fields...)
}

func Debug(message string, fields ...zap.Field) {
	logger.Debug(message, fields...)
}

func Error(message string, fields ...zap.Field) {
	logger.Error(message, fields...)
}

func Fatal(message string, fields ...zap.Field) {
	logger.Fatal(message, fields...)
}

func With(fields ...zap.Field) *zap.Logger {
	return logger.With(fields...)
}
