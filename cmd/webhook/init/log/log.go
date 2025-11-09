package log

import (
	"log/slog"
	"os"
)

var logger *slog.Logger

func Init() {
	// Set the log level
	var level slog.Level
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Set the log format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "test" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)
}

func Debug(message string, args ...any) {
	logger.Debug(message, args...)
}

func Info(message string, args ...any) {
	logger.Info(message, args...)
}

func Warn(message string, args ...any) {
	logger.Warn(message, args...)
}

func Error(message string, args ...any) {
	logger.Error(message, args...)
}

func Fatal(message string, args ...any) {
	logger.Error(message, args...)
	os.Exit(1)
}

func With(args ...any) *slog.Logger {
	return logger.With(args...)
}
