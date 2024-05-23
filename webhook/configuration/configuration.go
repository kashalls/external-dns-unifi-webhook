package configuration

import (
	"time"

	"github.com/caarlos0/env/v8"
	log "github.com/sirupsen/logrus"
)

// Config struct for configuration environmental variables
type Config struct {
	UnifiHost          string        `env:"UNIFI_HOST"`
	UnifiUser          string        `env:"UNIFI_USER" envDefault:"external-dns-unifi"`
	UnifiPass          string        `env:"UNIFI_PASS" envDefault:"V3ryS3cur3!!"`
	UnifiSkipTLSVerify bool          `env:"UNIFI_SKIP_TLS_VERIFY" envDefault:"true"`
	ServerHost         string        `env:"SERVER_HOST" envDefault:"localhost"`
	ServerPort         int           `env:"SERVER_PORT" envDefault:"8888"`
	ServerReadTimeout  time.Duration `env:"SERVER_READ_TIMEOUT"`
	ServerWriteTimeout time.Duration `env:"SERVER_WRITE_TIMEOUT"`
}

// Init sets up configuration by reading set environmental variables
func Init() Config {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("Error reading configuration from environment: %v", err)
	}
	return cfg
}
