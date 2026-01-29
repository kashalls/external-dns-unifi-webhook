package unifi

import (
	"sigs.k8s.io/external-dns/endpoint"
)

// Config represents the configuration for the UniFi API.
type Config struct {
	Host               string `env:"UNIFI_HOST,notEmpty"`
	APIKey             string `env:"UNIFI_API_KEY"             envDefault:""`
	User               string `env:"UNIFI_USER"                envDefault:""`
	Password           string `env:"UNIFI_PASS"                envDefault:""`
	Site               string `env:"UNIFI_SITE"                envDefault:"default"`
	ExternalController bool   `env:"UNIFI_EXTERNAL_CONTROLLER" envDefault:"false"` // If false, Network Controller is on device
	SkipTLSVerify      bool   `env:"UNIFI_SKIP_TLS_VERIFY"     envDefault:"true"`
	UseExperimentalAPI bool   `env:"UNIFI_EXPERIMENTAL_API"    envDefault:"false"`
	UseCloudConnector  bool   `env:"UNIFI_CLOUD_CONNECTOR"     envDefault:"false"` // https://developer.ui.com/network/v10.1.68/connectorget
	ConsoleID          string `env:"UNIFI_CLOUD_CONSOLE_ID"    envDefault:""`
}

// Login represents a login request to the UniFi API.
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
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
