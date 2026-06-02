package unifi

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"sigs.k8s.io/external-dns/endpoint"
)

// Config represents the configuration for the UniFi API.
type Config struct {
	Host              string        `env:"UNIFI_HOST,notEmpty"`
	APIKey            string        `env:"UNIFI_API_KEY,notEmpty,unset"`
	Site              string        `env:"UNIFI_SITE"                envDefault:"default"`
	ConsoleID         string        `env:"UNIFI_CONSOLE_ID"          envDefault:""`
	SkipTLSVerify     bool          `env:"UNIFI_SKIP_TLS_VERIFY"     envDefault:"true"`
	CACertPath        string        `env:"UNIFI_CA_CERT"             envDefault:""`
	RetryAttempts     int           `env:"UNIFI_RETRY_ATTEMPTS"      envDefault:"3"`
	RetryInitialDelay time.Duration `env:"UNIFI_RETRY_INITIAL_DELAY" envDefault:"500ms"`
	RetryMaxDelay     time.Duration `env:"UNIFI_RETRY_MAX_DELAY"     envDefault:"10s"`
	ApplyWorkers      int           `env:"UNIFI_APPLY_WORKERS"       envDefault:"5"`
}

// isCloud reports whether requests route through the Ubiquiti cloud connector
// (api.ui.com) rather than directly to a local controller. A console ID is the
// operator's explicit opt-in to the cloud path.
func (c Config) isCloud() bool { return c.ConsoleID != "" }

// baseURL returns the URL prefix that every Integration API path is appended
// to. In cloud mode the request is routed through api.ui.com's per-console
// connector proxy; otherwise it hits the controller's own reverse proxy. The
// Integration API surface past this prefix is identical for both.
func (c Config) baseURL() string {
	host := strings.TrimRight(c.Host, "/")
	if !c.isCloud() {
		return host
	}

	return host + "/v1/connector/consoles/" + c.ConsoleID
}

// cloudAPIHosts are the Ubiquiti cloud connector endpoints. Pointing UNIFI_HOST
// at one of these without a console ID can't work — the connector needs to know
// which console to proxy to.
var cloudAPIHosts = map[string]bool{
	"api.ui.com":     true,
	"api.dev.ui.com": true,
	"api.stg.ui.com": true,
}

// Validate checks that the UniFi configuration is internally consistent. It is
// called once at provider construction so misconfiguration fails startup
// rather than surfacing as confusing request-time errors.
func (c Config) Validate() error {
	u, err := url.Parse(c.Host)
	if err != nil {
		return fmt.Errorf("UNIFI_HOST %q is not a valid URL: %w", c.Host, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("UNIFI_HOST %q must include a scheme and host (e.g. https://unifi.local)", c.Host)
	}
	if cloudAPIHosts[u.Host] && c.ConsoleID == "" {
		return fmt.Errorf("UNIFI_CONSOLE_ID is required when UNIFI_HOST points at the Ubiquiti cloud API (%q)", u.Host)
	}
	if c.RetryAttempts < 1 {
		return fmt.Errorf("UNIFI_RETRY_ATTEMPTS must be at least 1, got %d", c.RetryAttempts)
	}
	if c.ApplyWorkers < 1 {
		return fmt.Errorf("UNIFI_APPLY_WORKERS must be at least 1, got %d", c.ApplyWorkers)
	}
	if c.RetryInitialDelay <= 0 {
		return fmt.Errorf("UNIFI_RETRY_INITIAL_DELAY must be positive, got %s", c.RetryInitialDelay)
	}
	if c.RetryMaxDelay < c.RetryInitialDelay {
		return fmt.Errorf("UNIFI_RETRY_MAX_DELAY (%s) must be >= UNIFI_RETRY_INITIAL_DELAY (%s)",
			c.RetryMaxDelay, c.RetryInitialDelay)
	}

	return nil
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
