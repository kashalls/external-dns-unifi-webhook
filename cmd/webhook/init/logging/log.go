package logging

import (
	"os"

	log "github.com/sirupsen/logrus"
)

func Init() {
	setLogLevel()
	setLogFormat()
}

func setLogFormat() {
	format := os.Getenv("LOG_FORMAT")
	if format == "test" {
		log.SetFormatter(&log.TextFormatter{})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}
}

func setLogLevel() {
	level := os.Getenv("LOG_LEVEL")
	switch level {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}
