package unifi

import "sigs.k8s.io/external-dns/endpoint"

// Config holds configuration from environmental variables
type Config struct {
	Host          string `env:"UNIFI_HOST,notEmpty"`
	User          string `env:"UNIFI_USER,notEmpty"`
	Password      string `env:"UNIFI_PASS,notEmpty"`
	SkipTLSVerify bool   `env:"UNIFI_SKIP_TLS_VERIFY" envDefault:"true"`
}

// DNSRecord represents a DNS record in the UniFi API.
type DNSRecord struct {
	ID         string       `json:"_id,omitempty"`
	Enabled    bool         `json:"enabled,omitempty"`
	Key        string       `json:"key"`
	Port       int          `json:"port,omitempty"`
	Priority   int          `json:"priority,omitempty"`
	RecordType string       `json:"record_type"`
	TTL        endpoint.TTL `json:"ttl,omitempty"`
	Value      string       `json:"value"`
	Weight     int          `json:"weight,omitempty"`
}
