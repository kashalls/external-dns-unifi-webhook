package unifi

import (
	"time"

	"sigs.k8s.io/external-dns/endpoint"
)

// Config represents the configuration for the UniFi API.
type Config struct {
	Host               string        `env:"UNIFI_HOST,notEmpty"`
	APIKey             string        `env:"UNIFI_API_KEY,notEmpty"`
	Site               string        `env:"UNIFI_SITE"                envDefault:"default"`
	ExternalController bool          `env:"UNIFI_EXTERNAL_CONTROLLER" envDefault:"false"`
	SkipTLSVerify      bool          `env:"UNIFI_SKIP_TLS_VERIFY"     envDefault:"true"`
	CACertPath         string        `env:"UNIFI_CA_CERT"             envDefault:""`
	RetryAttempts      int           `env:"UNIFI_RETRY_ATTEMPTS"      envDefault:"3"`
	RetryInitialDelay  time.Duration `env:"UNIFI_RETRY_INITIAL_DELAY" envDefault:"500ms"`
	RetryMaxDelay      time.Duration `env:"UNIFI_RETRY_MAX_DELAY"     envDefault:"10s"`
	ApplyWorkers       int           `env:"UNIFI_APPLY_WORKERS"       envDefault:"5"`
}

// DNSRecord represents a DNS record in the UniFi API.
//
//nolint:tagliatelle // UniFi API field names cannot be changed
type DNSRecord struct {
	ID         string       `json:"_id,omitempty"`
	Enabled    bool         `json:"enabled,omitempty"`
	Key        string       `json:"key"`
	Port       *int         `json:"port,omitempty"`
	Priority   *int         `json:"priority,omitempty"`
	RecordType string       `json:"record_type"`
	TTL        endpoint.TTL `json:"ttl,omitempty"`
	Value      string       `json:"value"`
	Weight     *int         `json:"weight,omitempty"`
}

//nolint:revive // UnifiErrorResponse matches UniFi API naming conventions
type UnifiErrorResponse struct {
	Code      string         `json:"code"`
	Details   map[string]any `json:"details"`
	ErrorCode int            `json:"errorCode"`
	Message   string         `json:"message"`
}
