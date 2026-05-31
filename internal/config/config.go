package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds configuration loaded from the process environment.
//
// Domain filters (DOMAIN_FILTER, REGEXP_DOMAIN_FILTER, etc.) are deliberately
// not exposed here. UniFi has no zones for the provider to filter against;
// echoing the operator's intent back via GetDomainFilter is the anti-pattern
// documented in sigs.k8s.io/external-dns/docs/contributing/sources-and-providers.md
// (PR #6249). Configure --domain-filter on the external-dns controller instead.
type Config struct {
	ServerHost              string        `env:"SERVER_HOST"                envDefault:"localhost"`
	ServerPort              int           `env:"SERVER_PORT"                envDefault:"8888"`
	ServerReadTimeout       time.Duration `env:"SERVER_READ_TIMEOUT"        envDefault:"60s"`
	ServerReadHeaderTimeout time.Duration `env:"SERVER_READ_HEADER_TIMEOUT" envDefault:"5s"`
	ServerWriteTimeout      time.Duration `env:"SERVER_WRITE_TIMEOUT"       envDefault:"60s"`
	ServerIdleTimeout       time.Duration `env:"SERVER_IDLE_TIMEOUT"        envDefault:"120s"`
	ServerMaxHeaderBytes    int           `env:"SERVER_MAX_HEADER_BYTES"    envDefault:"65536"`
	ServerMaxBodyBytes      int64         `env:"SERVER_MAX_BODY_BYTES"      envDefault:"5242880"`
	HealthServerAddr        string        `env:"HEALTH_SERVER_ADDR"         envDefault:"0.0.0.0:8080"`
	ReadinessCacheTTL       time.Duration `env:"READINESS_CACHE_TTL"        envDefault:"30s"`
	PprofEnabled            bool          `env:"PPROF_ENABLED"              envDefault:"false"`
}

// Init reads Config from the environment. Parse errors fail startup.
func Init() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("reading configuration from environment: %w", err)
	}

	return cfg, nil
}
