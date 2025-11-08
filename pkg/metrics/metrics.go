package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace    = "externaldns_webhook"
	ProviderName = "unifi"
)

// Metrics holds all Prometheus metrics for the webhook
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal          *prometheus.CounterVec
	HTTPRequestDuration        *prometheus.HistogramVec
	HTTPRequestsInFlight       *prometheus.GaugeVec
	HTTPResponseSizeBytes      *prometheus.HistogramVec
	HTTPValidationErrorsTotal  *prometheus.CounterVec
	HTTPJSONErrorsTotal        *prometheus.CounterVec

	// Business metrics - DNS records
	RecordsTotal               *prometheus.GaugeVec
	ChangesTotal               *prometheus.CounterVec
	ChangesByTypeTotal         *prometheus.CounterVec
	CNAMEConflictsTotal        *prometheus.CounterVec
	IgnoredCNAMETargetsTotal   *prometheus.CounterVec
	SRVParsingErrorsTotal      *prometheus.CounterVec
	BatchSize                  *prometheus.HistogramVec

	// Endpoint operations
	AdjustEndpointsTotal       *prometheus.CounterVec
	NegotiateTotal             *prometheus.CounterVec

	// UniFi API metrics
	UniFiAPIErrorsTotal        *prometheus.CounterVec
	UniFiAPIDuration           *prometheus.HistogramVec
	UniFiLoginTotal            *prometheus.CounterVec
	UniFiReloginTotal          *prometheus.CounterVec
	UniFiCSRFRefreshesTotal    *prometheus.CounterVec
	UniFiConnected             *prometheus.GaugeVec
	UniFiResponseSizeBytes     *prometheus.HistogramVec

	// Quality metrics
	ConsecutiveErrors          *prometheus.GaugeVec
	LastSuccessTimestamp       *prometheus.GaugeVec
	OperationSuccessRate       *prometheus.GaugeVec

	// Info metric
	Info                       *prometheus.GaugeVec
}

var instance *Metrics

// New creates and registers all metrics
func New(version string) *Metrics {
	if instance != nil {
		return instance
	}

	m := &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"provider", "method", "endpoint", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"provider", "method", "endpoint"},
		),
		HTTPRequestsInFlight: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being processed",
			},
			[]string{"provider"},
		),
		HTTPResponseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"provider", "method", "endpoint"},
		),
		HTTPValidationErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_validation_errors_total",
				Help:      "Total number of HTTP header validation errors",
			},
			[]string{"provider", "header_type"},
		),
		HTTPJSONErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_json_errors_total",
				Help:      "Total number of JSON decoding errors",
			},
			[]string{"provider", "endpoint"},
		),

		// Business metrics
		RecordsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "records_total",
				Help:      "Total number of DNS records by type",
			},
			[]string{"provider", "record_type"},
		),
		ChangesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "changes_total",
				Help:      "Total number of DNS changes",
			},
			[]string{"provider", "operation"},
		),
		ChangesByTypeTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "changes_by_type_total",
				Help:      "Total number of DNS changes by record type",
			},
			[]string{"provider", "operation", "record_type"},
		),
		CNAMEConflictsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cname_conflicts_total",
				Help:      "Total number of CNAME conflicts detected",
			},
			[]string{"provider"},
		),
		IgnoredCNAMETargetsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ignored_cname_targets_total",
				Help:      "Total number of ignored CNAME targets (only first target is used)",
			},
			[]string{"provider"},
		),
		SRVParsingErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "srv_parsing_errors_total",
				Help:      "Total number of SRV record parsing errors",
			},
			[]string{"provider"},
		),
		BatchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "batch_size",
				Help:      "Size of change batches",
				Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"provider", "operation"},
		),

		// Endpoint operations
		AdjustEndpointsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "adjust_endpoints_total",
				Help:      "Total number of adjust endpoints calls",
			},
			[]string{"provider"},
		),
		NegotiateTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "negotiate_total",
				Help:      "Total number of negotiate calls",
			},
			[]string{"provider"},
		),

		// UniFi API metrics
		UniFiAPIErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_api_errors_total",
				Help:      "Total number of UniFi API errors",
			},
			[]string{"provider", "operation"},
		),
		UniFiAPIDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "unifi_api_duration_seconds",
				Help:      "UniFi API request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"provider", "operation"},
		),
		UniFiLoginTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_login_total",
				Help:      "Total number of UniFi login attempts",
			},
			[]string{"provider", "status"},
		),
		UniFiReloginTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_relogin_total",
				Help:      "Total number of UniFi re-login attempts after 401",
			},
			[]string{"provider"},
		),
		UniFiCSRFRefreshesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_csrf_refreshes_total",
				Help:      "Total number of CSRF token refreshes",
			},
			[]string{"provider"},
		),
		UniFiConnected: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "unifi_connected",
				Help:      "UniFi connection status (1 = connected, 0 = disconnected)",
			},
			[]string{"provider"},
		),
		UniFiResponseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "unifi_response_size_bytes",
				Help:      "UniFi API response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"operation"},
		),

		// Quality metrics
		ConsecutiveErrors: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "consecutive_errors",
				Help:      "Number of consecutive errors",
			},
			[]string{"provider"},
		),
		LastSuccessTimestamp: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "last_success_timestamp",
				Help:      "Timestamp of last successful operation",
			},
			[]string{"provider"},
		),
		OperationSuccessRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "operation_success_rate",
				Help:      "Success rate of operations (0-1)",
			},
			[]string{"operation"},
		),

		// Info metric
		Info: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "info",
				Help:      "Information about the webhook instance",
			},
			[]string{"version", "provider"},
		),
	}

	// Set info metric
	m.Info.WithLabelValues(version, ProviderName).Set(1)

	instance = m
	return m
}

// Get returns the singleton metrics instance
func Get() *Metrics {
	if instance == nil {
		return New("unknown")
	}
	return instance
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, endpoint string, statusCode int, duration time.Duration, responseSize int) {
	m.HTTPRequestsTotal.WithLabelValues(ProviderName, method, endpoint, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(ProviderName, method, endpoint).Observe(duration.Seconds())
	if responseSize > 0 {
		m.HTTPResponseSizeBytes.WithLabelValues(ProviderName, method, endpoint).Observe(float64(responseSize))
	}
}

// RecordUniFiAPICall records UniFi API call metrics
func (m *Metrics) RecordUniFiAPICall(operation string, duration time.Duration, responseSize int, err error) {
	m.UniFiAPIDuration.WithLabelValues(ProviderName, operation).Observe(duration.Seconds())
	if responseSize > 0 {
		m.UniFiResponseSizeBytes.WithLabelValues(operation).Observe(float64(responseSize))
	}
	if err != nil {
		m.UniFiAPIErrorsTotal.WithLabelValues(ProviderName, operation).Inc()
		m.ConsecutiveErrors.WithLabelValues(ProviderName).Inc()
	} else {
		m.ConsecutiveErrors.WithLabelValues(ProviderName).Set(0)
		m.LastSuccessTimestamp.WithLabelValues(ProviderName).Set(float64(time.Now().Unix()))
	}
}

// UpdateRecordsByType updates the records count by type
func (m *Metrics) UpdateRecordsByType(recordType string, count int) {
	m.RecordsTotal.WithLabelValues(ProviderName, recordType).Set(float64(count))
}

// RecordChange records a DNS change operation
func (m *Metrics) RecordChange(operation, recordType string) {
	m.ChangesTotal.WithLabelValues(ProviderName, operation).Inc()
	m.ChangesByTypeTotal.WithLabelValues(ProviderName, operation, recordType).Inc()
}
