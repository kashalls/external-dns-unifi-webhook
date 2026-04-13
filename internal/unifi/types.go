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
	UseCloudConnector  bool   `env:"UNIFI_CLOUD_CONNECTOR"     envDefault:"false"` // https://developer.ui.com/network/v10.1.68/connectorget
	ConsoleID          string `env:"UNIFI_CLOUD_CONSOLE_ID"    envDefault:""`
	UseIntegrationAPI  bool   `env:"UNIFI_INTEGRATION_API"     envDefault:"false"` // Use the new /proxy/network/integration/v1 API
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

// DNSPolicy represents a DNS policy entry in the UniFi Integration API
// (GET/POST/PUT /proxy/network/integration/v1/sites/{siteId}/dns/policies).
// All record-type-specific fields are optional pointers; only the fields
// relevant to the discriminated Type value will be populated.
type DNSPolicy struct {
	ID               string  `json:"id,omitempty"`
	Type             string  `json:"type"`
	Enabled          bool    `json:"enabled"`
	Domain           string  `json:"domain,omitempty"`
	IPv4Address      *string `json:"ipv4Address,omitempty"`
	IPv6Address      *string `json:"ipv6Address,omitempty"`
	TTLSeconds       *int    `json:"ttlSeconds,omitempty"`
	TargetDomain     *string `json:"targetDomain,omitempty"`
	MailServerDomain *string `json:"mailServerDomain,omitempty"`
	Priority         *int    `json:"priority,omitempty"`
	Text             *string `json:"text,omitempty"`
	ServerDomain     *string `json:"serverDomain,omitempty"`
	Service          *string `json:"service,omitempty"`
	Protocol         *string `json:"protocol,omitempty"`
	Port             *int    `json:"port,omitempty"`
	Weight           *int    `json:"weight,omitempty"`
	IPAddress        *string `json:"ipAddress,omitempty"`
}

// DNSPolicyPage is the paginated list response from the Integration API list endpoint.
type DNSPolicyPage struct {
	Offset     int64       `json:"offset"`
	Limit      int32       `json:"limit"`
	Count      int32       `json:"count"`
	TotalCount int64       `json:"totalCount"`
	Data       []DNSPolicy `json:"data"`
}

// IntegrationSite is a single site entry returned by the Integration API sites list.
type IntegrationSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IntegrationSitePage is the paginated response from GET /proxy/network/integration/v1/sites.
type IntegrationSitePage struct {
	Offset     int64             `json:"offset"`
	Limit      int32             `json:"limit"`
	Count      int32             `json:"count"`
	TotalCount int64             `json:"totalCount"`
	Data       []IntegrationSite `json:"data"`
}
