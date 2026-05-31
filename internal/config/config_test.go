package config

import (
	"testing"
	"time"
)

// validConfig returns a Config that passes Validate; tests mutate one field at
// a time to exercise each rule.
func validConfig() Config {
	return Config{
		ServerPort:           8888,
		ServerMaxBodyBytes:   5 << 20,
		ServerMaxHeaderBytes: 1 << 16,
		ServerReadTimeout:    60 * time.Second,
		HealthServerAddr:     "0.0.0.0:8080",
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(*Config) {}, false},
		{"port zero", func(c *Config) { c.ServerPort = 0 }, true},
		{"port too high", func(c *Config) { c.ServerPort = 70000 }, true},
		{"body bytes zero", func(c *Config) { c.ServerMaxBodyBytes = 0 }, true},
		{"header bytes negative", func(c *Config) { c.ServerMaxHeaderBytes = -1 }, true},
		{"negative timeout", func(c *Config) { c.ServerWriteTimeout = -time.Second }, true},
		{"bad health addr", func(c *Config) { c.HealthServerAddr = "not-host-port" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr != (err != nil) {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestInit_Defaults(t *testing.T) {
	t.Setenv("SERVER_HOST", "")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("HEALTH_SERVER_ADDR", "")

	cfg, err := Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if cfg.ServerHost != "localhost" {
		t.Errorf("ServerHost = %q, want localhost", cfg.ServerHost)
	}
	if cfg.ServerPort != 8888 {
		t.Errorf("ServerPort = %d, want 8888", cfg.ServerPort)
	}
	if cfg.HealthServerAddr != "0.0.0.0:8080" {
		t.Errorf("HealthServerAddr = %q, want 0.0.0.0:8080", cfg.HealthServerAddr)
	}
}

func TestInit_ParseErrorPropagates(t *testing.T) {
	t.Setenv("SERVER_PORT", "not-a-number")

	_, err := Init()
	if err == nil {
		t.Fatal("Init() with invalid SERVER_PORT returned nil error")
	}
}
