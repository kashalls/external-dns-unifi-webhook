package config

import (
	"testing"
)

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
