package unifi

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// HTTPTransport defines the interface for low-level HTTP communication with UniFi controller.
// It handles authentication, CSRF tokens, and raw HTTP requests.
type HTTPTransport interface {
	// DoRequest performs an HTTP request with automatic retry on 401 and CSRF token management.
	DoRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)

	// Login authenticates with the UniFi controller using username/password (deprecated).
	Login(ctx context.Context) error

	// SetHeaders sets the required headers for UniFi API requests (API key or CSRF token).
	SetHeaders(req *http.Request)
}

// UnifiAPI defines the interface for interacting with UniFi DNS API.
// This is the main abstraction that consumers should depend on.
//
//nolint:revive // UnifiAPI is intentionally named to clarify it's the UniFi-specific API interface
type UnifiAPI interface {
	// GetEndpoints retrieves all DNS records from the UniFi controller.
	GetEndpoints(ctx context.Context) ([]DNSRecord, error)

	// CreateEndpoint creates a new DNS record in the UniFi controller.
	// Returns the created records (multiple for multi-target records like A/AAAA).
	CreateEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) ([]*DNSRecord, error)

	// DeleteEndpoint deletes a DNS record from the UniFi controller.
	DeleteEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) error
}

// RecordTransformer defines the interface for transforming DNS records between
// external-dns and UniFi formats.
type RecordTransformer interface {
	// PrepareDNSRecord creates a DNSRecord from an external-dns endpoint and target value.
	PrepareDNSRecord(endpoint *externaldnsendpoint.Endpoint, target string) DNSRecord

	// ParseSRVTarget parses SRV record format (priority weight port target) and
	// populates the Priority, Weight, Port fields of the DNSRecord.
	ParseSRVTarget(record *DNSRecord, target string) error

	// FormatSRVValue formats SRV record fields into the target string format
	// used by external-dns (priority weight port target).
	FormatSRVValue(priority, weight, port int, target string) string
}

// MetricsRecorder defines the interface for recording UniFi-specific metrics.
//
//nolint:interfacebloat // MetricsRecorder needs to expose all UniFi metrics for complete observability
type MetricsRecorder interface {
	// RecordUniFiAPICall records metrics for a UniFi API call.
	RecordUniFiAPICall(operation string, duration time.Duration, responseSize int, err error)

	// RecordChange records a DNS change operation.
	RecordChange(operation, recordType string)

	// UpdateRecordsByType updates the records count by type.
	UpdateRecordsByType(recordType string, count int)

	// Prometheus metrics for direct manipulation
	IgnoredCNAMETargetsTotal() prometheus.Counter
	SRVParsingErrorsTotal() prometheus.Counter
	CNAMEConflictsTotal() *prometheus.CounterVec
	BatchSize() *prometheus.HistogramVec
	UniFiLoginTotal() *prometheus.CounterVec
	UniFiConnected() *prometheus.GaugeVec
	UniFiCSRFRefreshesTotal() *prometheus.CounterVec
	UniFiReloginTotal() *prometheus.CounterVec
}

// Logger defines the interface for structured logging.
type Logger interface {
	// Debug logs a debug message with optional key-value pairs.
	Debug(msg string, keysAndValues ...any)

	// Info logs an info message with optional key-value pairs.
	Info(msg string, keysAndValues ...any)

	// Warn logs a warning message with optional key-value pairs.
	Warn(msg string, keysAndValues ...any)

	// Error logs an error message with optional key-value pairs.
	Error(msg string, keysAndValues ...any)
}
