package unifi

import (
	"testing"
	"time"
)

// validUnifiConfig returns a Config that passes Validate; tests mutate one
// field at a time to exercise each rule.
func validUnifiConfig() Config {
	return Config{
		Host:              testHost,
		APIKey:            testKeyA,
		Site:              testSite,
		RetryAttempts:     3,
		RetryInitialDelay: 500 * time.Millisecond,
		RetryMaxDelay:     10 * time.Second,
		ApplyWorkers:      5,
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(*Config) {}, false},
		{"host missing scheme", func(c *Config) { c.Host = "unifi.local" }, true},
		{"host empty", func(c *Config) { c.Host = "" }, true},
		{"retry attempts zero", func(c *Config) { c.RetryAttempts = 0 }, true},
		{"apply workers zero", func(c *Config) { c.ApplyWorkers = 0 }, true},
		{"initial delay zero", func(c *Config) { c.RetryInitialDelay = 0 }, true},
		{"max delay below initial", func(c *Config) { c.RetryMaxDelay = 100 * time.Millisecond }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validUnifiConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr != (err != nil) {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
