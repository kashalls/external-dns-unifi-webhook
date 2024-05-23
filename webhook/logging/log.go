package logging

import (
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func Init() {
	setLogLevel()
	setLogFormat()
}

func setLogFormat() {
	format := os.Getenv("LOG_FORMAT")
	if format == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{})
	}
}

func setLogLevel() {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		log.SetLevel(log.InfoLevel)
	} else {
		if levelInt, err := strconv.Atoi(level); err == nil {
			log.SetLevel(log.Level(uint32(levelInt)))
		} else {
			levelInt, err := log.ParseLevel(level)
			if err != nil {
				log.SetLevel(log.InfoLevel)
				log.Errorf("Invalid log level '%s', defaulting to info", level)
			} else {
				log.SetLevel(levelInt)
			}
		}
	}
}
