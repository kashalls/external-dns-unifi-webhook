package unifi

import (
	"time"

	"sigs.k8s.io/external-dns/endpoint"
)

// Config represents the configuration for the UniFi API.
type Config struct {
	Host              string        `env:"UNIFI_HOST,notEmpty"`
	APIKey            string        `env:"UNIFI_API_KEY,notEmpty"`
	Site              string        `env:"UNIFI_SITE"                envDefault:"default"`
	SkipTLSVerify     bool          `env:"UNIFI_SKIP_TLS_VERIFY"     envDefault:"true"`
	CACertPath        string        `env:"UNIFI_CA_CERT"             envDefault:""`
	RetryAttempts     int           `env:"UNIFI_RETRY_ATTEMPTS"      envDefault:"3"`
	RetryInitialDelay time.Duration `env:"UNIFI_RETRY_INITIAL_DELAY" envDefault:"500ms"`
	RetryMaxDelay     time.Duration `env:"UNIFI_RETRY_MAX_DELAY"     envDefault:"10s"`
	ApplyWorkers      int           `env:"UNIFI_APPLY_WORKERS"       envDefault:"5"`
}

// DNSRecord is the internal representation of a record used by the provider
// layer. Wire-level type-switching (per record type DTOs, SRV/MX field splits)
// happens in dto.go; the provider only ever sees this shape.
//
// For SRV records, Value carries "priority weight port serverDomain".
// For MX records, Value carries "priority mailServerDomain".
// For all other types, Value is the raw target.
type DNSRecord struct {
	ID         string
	Enabled    bool
	Key        string
	RecordType string
	TTL        endpoint.TTL
	Value      string
}

// apiErrorResponse matches the Integration API error envelope.
//
//nolint:tagliatelle // UniFi API field names cannot be changed
type apiErrorResponse struct {
	StatusCode  int    `json:"statusCode"`
	StatusName  string `json:"statusName"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Timestamp   string `json:"timestamp"`
	RequestPath string `json:"requestPath"`
	RequestID   string `json:"requestId"`
}
