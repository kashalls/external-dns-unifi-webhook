package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "externaldns_webhook"
)

// Metrics holds all Prometheus metrics for the webhook
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal          *prometheus.CounterVec
	HTTPRequestDuration        *prometheus.HistogramVec
	HTTPRequestsInFlight       prometheus.Gauge
	HTTPResponseSizeBytes      *prometheus.HistogramVec
	HTTPValidationErrorsTotal  *prometheus.CounterVec
	HTTPJSONErrorsTotal        *prometheus.CounterVec

	// Business metrics - DNS records
	RecordsTotal               *prometheus.GaugeVec
	ChangesTotal               *prometheus.CounterVec
	ChangesByTypeTotal         *prometheus.CounterVec
	CNAMEConflictsTotal        prometheus.Counter
	IgnoredCNAMETargetsTotal   prometheus.Counter
	SRVParsingErrorsTotal      prometheus.Counter
	BatchSize                  *prometheus.HistogramVec

	// Endpoint operations
	AdjustEndpointsTotal       prometheus.Counter
	NegotiateTotal             prometheus.Counter

	// UniFi API metrics
	UniFiAPIErrorsTotal        *prometheus.CounterVec
	UniFiAPIDuration           *prometheus.HistogramVec
	UniFiLoginTotal            *prometheus.CounterVec
	UniFiReloginTotal          prometheus.Counter
	UniFiCSRFRefreshesTotal    prometheus.Counter
	UniFiConnected             prometheus.Gauge
	UniFiResponseSizeBytes     *prometheus.HistogramVec

	// Quality metrics
	ConsecutiveErrors          prometheus.Gauge
	LastSuccessTimestamp       prometheus.Gauge
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
			[]string{"method", "endpoint", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being processed",
			},
		),
		HTTPResponseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "endpoint"},
		),
		HTTPValidationErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_validation_errors_total",
				Help:      "Total number of HTTP header validation errors",
			},
			[]string{"header_type"},
		),
		HTTPJSONErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_json_errors_total",
				Help:      "Total number of JSON decoding errors",
			},
			[]string{"endpoint"},
		),

		// Business metrics
		RecordsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "records_total",
				Help:      "Total number of DNS records by type",
			},
			[]string{"record_type"},
		),
		ChangesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "changes_total",
				Help:      "Total number of DNS changes",
			},
			[]string{"operation"},
		),
		ChangesByTypeTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "changes_by_type_total",
				Help:      "Total number of DNS changes by record type",
			},
			[]string{"operation", "record_type"},
		),
		CNAMEConflictsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cname_conflicts_total",
				Help:      "Total number of CNAME conflicts detected",
			},
		),
		IgnoredCNAMETargetsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ignored_cname_targets_total",
				Help:      "Total number of ignored CNAME targets (only first target is used)",
			},
		),
		SRVParsingErrorsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "srv_parsing_errors_total",
				Help:      "Total number of SRV record parsing errors",
			},
		),
		BatchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "batch_size",
				Help:      "Size of change batches",
				Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"operation"},
		),

		// Endpoint operations
		AdjustEndpointsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "adjust_endpoints_total",
				Help:      "Total number of adjust endpoints calls",
			},
		),
		NegotiateTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "negotiate_total",
				Help:      "Total number of negotiate calls",
			},
		),

		// UniFi API metrics
		UniFiAPIErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_api_errors_total",
				Help:      "Total number of UniFi API errors",
			},
			[]string{"operation"},
		),
		UniFiAPIDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "unifi_api_duration_seconds",
				Help:      "UniFi API request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		UniFiLoginTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_login_total",
				Help:      "Total number of UniFi login attempts",
			},
			[]string{"status"},
		),
		UniFiReloginTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_relogin_total",
				Help:      "Total number of UniFi re-login attempts after 401",
			},
		),
		UniFiCSRFRefreshesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "unifi_csrf_refreshes_total",
				Help:      "Total number of CSRF token refreshes",
			},
		),
		UniFiConnected: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "unifi_connected",
				Help:      "UniFi connection status (1 = connected, 0 = disconnected)",
			},
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
		ConsecutiveErrors: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "consecutive_errors",
				Help:      "Number of consecutive errors",
			},
		),
		LastSuccessTimestamp: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "last_success_timestamp",
				Help:      "Timestamp of last successful operation",
			},
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
	m.Info.WithLabelValues(version, "unifi").Set(1)

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
	m.HTTPRequestsTotal.WithLabelValues(method, endpoint, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
	if responseSize > 0 {
		m.HTTPResponseSizeBytes.WithLabelValues(method, endpoint).Observe(float64(responseSize))
	}
}

// RecordUniFiAPICall records UniFi API call metrics
func (m *Metrics) RecordUniFiAPICall(operation string, duration time.Duration, responseSize int, err error) {
	m.UniFiAPIDuration.WithLabelValues(operation).Observe(duration.Seconds())
	if responseSize > 0 {
		m.UniFiResponseSizeBytes.WithLabelValues(operation).Observe(float64(responseSize))
	}
	if err != nil {
		m.UniFiAPIErrorsTotal.WithLabelValues(operation).Inc()
		m.ConsecutiveErrors.Inc()
	} else {
		m.ConsecutiveErrors.Set(0)
		m.LastSuccessTimestamp.Set(float64(time.Now().Unix()))
	}
}

// UpdateRecordsByType updates the records count by type
func (m *Metrics) UpdateRecordsByType(recordType string, count int) {
	m.RecordsTotal.WithLabelValues(recordType).Set(float64(count))
}

// RecordChange records a DNS change operation
func (m *Metrics) RecordChange(operation, recordType string) {
	m.ChangesTotal.WithLabelValues(operation).Inc()
	m.ChangesByTypeTotal.WithLabelValues(operation, recordType).Inc()
}
