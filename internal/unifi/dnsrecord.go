package unifi

import "sigs.k8s.io/external-dns/endpoint"

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
