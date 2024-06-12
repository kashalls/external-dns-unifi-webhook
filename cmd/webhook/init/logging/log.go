package logging

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
	logger, err = config.Build()
	if err != nil {
		panic(err)
	}

	// Ensure we flush any buffered log entries
	defer logger.Sync()
}

// GetLogger returns the initialized logger instance
func GetLogger() *zap.Logger {
	if logger == nil {
		Init() // Initialize if not already done
	}
	return logger
}
