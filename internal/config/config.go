package config

import (
	"fmt"
	"net"
	"time"

	"github.com/caarlos0/env/v11"
)

const maxPort = 65535

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

// Init reads Config from the environment. Parse and validation errors fail
// startup so misconfiguration is caught before any server is bound.
func Init() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("reading configuration from environment: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks that the loaded configuration is internally consistent.
func (c Config) Validate() error {
	if c.ServerPort < 1 || c.ServerPort > maxPort {
		return fmt.Errorf("SERVER_PORT must be between 1 and %d, got %d", maxPort, c.ServerPort)
	}
	if c.ServerMaxBodyBytes <= 0 {
		return fmt.Errorf("SERVER_MAX_BODY_BYTES must be positive, got %d", c.ServerMaxBodyBytes)
	}
	if c.ServerMaxHeaderBytes <= 0 {
		return fmt.Errorf("SERVER_MAX_HEADER_BYTES must be positive, got %d", c.ServerMaxHeaderBytes)
	}

	durations := []struct {
		name  string
		value time.Duration
	}{
		{"SERVER_READ_TIMEOUT", c.ServerReadTimeout},
		{"SERVER_READ_HEADER_TIMEOUT", c.ServerReadHeaderTimeout},
		{"SERVER_WRITE_TIMEOUT", c.ServerWriteTimeout},
		{"SERVER_IDLE_TIMEOUT", c.ServerIdleTimeout},
		{"READINESS_CACHE_TTL", c.ReadinessCacheTTL},
	}
	for _, d := range durations {
		if d.value < 0 {
			return fmt.Errorf("%s must not be negative, got %s", d.name, d.value)
		}
	}

	if _, _, err := net.SplitHostPort(c.HealthServerAddr); err != nil {
		return fmt.Errorf("HEALTH_SERVER_ADDR %q is not a valid host:port: %w", c.HealthServerAddr, err)
	}

	return nil
}
