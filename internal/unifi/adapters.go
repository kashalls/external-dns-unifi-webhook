package unifi

import (
	"time"

	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metricsAdapter adapts the metrics package to the MetricsRecorder interface.
type metricsAdapter struct {
	m *metrics.Metrics
}

// NewMetricsAdapter creates a new metrics adapter.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func NewMetricsAdapter(m *metrics.Metrics) MetricsRecorder {
	return &metricsAdapter{m: m}
}

// RecordUniFiAPICall records metrics for a UniFi API call.
func (a *metricsAdapter) RecordUniFiAPICall(operation string, duration time.Duration, responseSize int, err error) {
	a.m.RecordUniFiAPICall(operation, duration, responseSize, err)
}

// RecordChange records a DNS change operation.
func (a *metricsAdapter) RecordChange(operation, recordType string) {
	a.m.RecordChange(operation, recordType)
}

// UpdateRecordsByType updates the records count by type.
func (a *metricsAdapter) UpdateRecordsByType(recordType string, count int) {
	a.m.UpdateRecordsByType(recordType, count)
}

// IgnoredCNAMETargetsTotal returns the ignored CNAME targets counter.
//
//nolint:ireturn // Prometheus Counter is an interface
func (a *metricsAdapter) IgnoredCNAMETargetsTotal() prometheus.Counter {
	return a.m.IgnoredCNAMETargetsTotal.WithLabelValues(metrics.ProviderName)
}

// SRVParsingErrorsTotal returns the SRV parsing errors counter.
//
//nolint:ireturn // Prometheus Counter is an interface
func (a *metricsAdapter) SRVParsingErrorsTotal() prometheus.Counter {
	return a.m.SRVParsingErrorsTotal.WithLabelValues(metrics.ProviderName)
}

// CNAMEConflictsTotal returns the CNAME conflicts counter.
func (a *metricsAdapter) CNAMEConflictsTotal() *prometheus.CounterVec {
	return a.m.CNAMEConflictsTotal
}

// BatchSize returns the batch size histogram.
func (a *metricsAdapter) BatchSize() *prometheus.HistogramVec {
	return a.m.BatchSize
}

// UniFiLoginTotal returns the UniFi login counter.
func (a *metricsAdapter) UniFiLoginTotal() *prometheus.CounterVec {
	return a.m.UniFiLoginTotal
}

// UniFiConnected returns the UniFi connected gauge.
func (a *metricsAdapter) UniFiConnected() *prometheus.GaugeVec {
	return a.m.UniFiConnected
}

// UniFiCSRFRefreshesTotal returns the CSRF refreshes counter.
func (a *metricsAdapter) UniFiCSRFRefreshesTotal() *prometheus.CounterVec {
	return a.m.UniFiCSRFRefreshesTotal
}

// UniFiReloginTotal returns the UniFi relogin counter.
func (a *metricsAdapter) UniFiReloginTotal() *prometheus.CounterVec {
	return a.m.UniFiReloginTotal
}

// loggerAdapter adapts the log package to the Logger interface.
type loggerAdapter struct{}

// NewLoggerAdapter creates a new logger adapter.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func NewLoggerAdapter() Logger {
	return &loggerAdapter{}
}

// Debug logs a debug message with optional key-value pairs.
func (l *loggerAdapter) Debug(msg string, keysAndValues ...any) {
	log.Debug(msg, keysAndValues...)
}

// Info logs an info message with optional key-value pairs.
func (l *loggerAdapter) Info(msg string, keysAndValues ...any) {
	log.Info(msg, keysAndValues...)
}

// Warn logs a warning message with optional key-value pairs.
func (l *loggerAdapter) Warn(msg string, keysAndValues ...any) {
	log.Warn(msg, keysAndValues...)
}

// Error logs an error message with optional key-value pairs.
func (l *loggerAdapter) Error(msg string, keysAndValues ...any) {
	log.Error(msg, keysAndValues...)
}
