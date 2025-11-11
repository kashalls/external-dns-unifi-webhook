package unifi

import (
	"testing"
	"time"

	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsAdapter_RecordUniFiAPICall(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	// Test successful call
	adapter.RecordUniFiAPICall("GetEndpoints", 100*time.Millisecond, 1024, nil)

	// Test failed call
	adapter.RecordUniFiAPICall("CreateEndpoint", 50*time.Millisecond, 0, NewAPIError("POST", "/api/records", 500, "internal error"))

	// Just verify it doesn't panic - actual metric verification would require prometheus test infrastructure
}

func TestMetricsAdapter_RecordChange(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	adapter.RecordChange("create", "A")
	adapter.RecordChange("delete", "CNAME")
	adapter.RecordChange("update", "TXT")

	// Just verify it doesn't panic
}

func TestMetricsAdapter_UpdateRecordsByType(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	adapter.UpdateRecordsByType("A", 10)
	adapter.UpdateRecordsByType("AAAA", 5)
	adapter.UpdateRecordsByType("CNAME", 3)
	adapter.UpdateRecordsByType("TXT", 2)
	adapter.UpdateRecordsByType("SRV", 1)

	// Just verify it doesn't panic
}

func TestMetricsAdapter_IgnoredCNAMETargetsTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counter := adapter.IgnoredCNAMETargetsTotal()
	if counter == nil {
		t.Fatal("IgnoredCNAMETargetsTotal() returned nil")
	}

	// Verify it's a valid Counter by calling Inc()
	counter.Inc()
}

func TestMetricsAdapter_SRVParsingErrorsTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counter := adapter.SRVParsingErrorsTotal()
	if counter == nil {
		t.Fatal("SRVParsingErrorsTotal() returned nil")
	}

	// Verify it's a valid Counter
	counter.Inc()
}

func TestMetricsAdapter_CNAMEConflictsTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counterVec := adapter.CNAMEConflictsTotal()
	if counterVec == nil {
		t.Fatal("CNAMEConflictsTotal() returned nil")
	}

	// Verify it's a valid CounterVec
	counterVec.WithLabelValues(metrics.ProviderName).Inc()
}

func TestMetricsAdapter_BatchSize(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	histogramVec := adapter.BatchSize()
	if histogramVec == nil {
		t.Fatal("BatchSize() returned nil")
	}

	// Verify it's a valid HistogramVec
	histogramVec.WithLabelValues(metrics.ProviderName, "create").Observe(5.0)
}

func TestMetricsAdapter_UniFiLoginTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counterVec := adapter.UniFiLoginTotal()
	if counterVec == nil {
		t.Fatal("UniFiLoginTotal() returned nil")
	}

	// Verify it's a valid CounterVec
	counterVec.WithLabelValues(metrics.ProviderName, "success").Inc()
}

func TestMetricsAdapter_UniFiConnected(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	gaugeVec := adapter.UniFiConnected()
	if gaugeVec == nil {
		t.Fatal("UniFiConnected() returned nil")
	}

	// Verify it's a valid GaugeVec
	gaugeVec.WithLabelValues(metrics.ProviderName).Set(1)
}

func TestMetricsAdapter_UniFiCSRFRefreshesTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counterVec := adapter.UniFiCSRFRefreshesTotal()
	if counterVec == nil {
		t.Fatal("UniFiCSRFRefreshesTotal() returned nil")
	}

	// Verify it's a valid CounterVec
	counterVec.WithLabelValues(metrics.ProviderName).Inc()
}

func TestMetricsAdapter_UniFiReloginTotal(t *testing.T) {
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	counterVec := adapter.UniFiReloginTotal()
	if counterVec == nil {
		t.Fatal("UniFiReloginTotal() returned nil")
	}

	// Verify it's a valid CounterVec
	counterVec.WithLabelValues(metrics.ProviderName).Inc()
}

func TestMetricsAdapter_AllMetrics(t *testing.T) {
	// Integration test to verify all metrics work together
	m := metrics.New("test")
	adapter := NewMetricsAdapter(m)

	// Record various operations
	adapter.RecordUniFiAPICall("GetEndpoints", 100*time.Millisecond, 2048, nil)
	adapter.RecordChange("create", "A")
	adapter.UpdateRecordsByType("A", 5)

	// Access all metric types
	adapter.IgnoredCNAMETargetsTotal().Inc()
	adapter.SRVParsingErrorsTotal().Inc()
	adapter.CNAMEConflictsTotal().WithLabelValues(metrics.ProviderName).Inc()
	adapter.BatchSize().WithLabelValues(metrics.ProviderName, "create").Observe(10.0)
	adapter.UniFiLoginTotal().WithLabelValues(metrics.ProviderName, "success").Inc()
	adapter.UniFiConnected().WithLabelValues(metrics.ProviderName).Set(1)
	adapter.UniFiCSRFRefreshesTotal().WithLabelValues(metrics.ProviderName).Inc()
	adapter.UniFiReloginTotal().WithLabelValues(metrics.ProviderName).Inc()

	// Verify that the adapter was created correctly
	if adapter == nil {
		t.Fatal("NewMetricsAdapter() returned nil")
	}
}

func TestLoggerAdapter_Debug(t *testing.T) {
	adapter := NewLoggerAdapter()

	// Test basic debug logging
	adapter.Debug("test message")
	adapter.Debug("test with values", "key", "value")
	adapter.Debug("test with multiple", "key1", "value1", "key2", 42)

	// Just verify it doesn't panic
}

func TestLoggerAdapter_Info(t *testing.T) {
	adapter := NewLoggerAdapter()

	// Test basic info logging
	adapter.Info("test message")
	adapter.Info("test with values", "key", "value")
	adapter.Info("operation complete", "duration", 100*time.Millisecond, "status", "success")

	// Just verify it doesn't panic
}

func TestLoggerAdapter_Warn(t *testing.T) {
	adapter := NewLoggerAdapter()

	// Test basic warn logging
	adapter.Warn("test warning")
	adapter.Warn("potential issue", "reason", "rate limit approaching")
	adapter.Warn("retry attempt", "attempt", 2, "maxRetries", 3)

	// Just verify it doesn't panic
}

func TestLoggerAdapter_Error(t *testing.T) {
	adapter := NewLoggerAdapter()

	// Test basic error logging
	adapter.Error("test error")
	adapter.Error("operation failed", "error", "connection timeout")
	adapter.Error("API error", "status", 500, "message", "internal server error")

	// Just verify it doesn't panic
}

func TestLoggerAdapter_AllLevels(t *testing.T) {
	// Integration test to verify all log levels work
	adapter := NewLoggerAdapter()

	adapter.Debug("debug message", "level", "debug")
	adapter.Info("info message", "level", "info")
	adapter.Warn("warn message", "level", "warn")
	adapter.Error("error message", "level", "error")

	// Verify that the adapter was created correctly
	if adapter == nil {
		t.Fatal("NewLoggerAdapter() returned nil")
	}
}

func TestAdapters_Integration(t *testing.T) {
	// Test that both adapters work together
	m := metrics.New("test")
	metricsAdapter := NewMetricsAdapter(m)
	loggerAdapter := NewLoggerAdapter()

	// Simulate a typical operation flow
	loggerAdapter.Info("starting operation")
	metricsAdapter.RecordUniFiAPICall("GetEndpoints", 150*time.Millisecond, 1024, nil)
	loggerAdapter.Debug("API call completed", "duration", "150ms")

	metricsAdapter.RecordChange("create", "A")
	loggerAdapter.Info("record created", "type", "A")

	metricsAdapter.UpdateRecordsByType("A", 10)

	// Verify adapters were created
	if metricsAdapter == nil {
		t.Fatal("metricsAdapter is nil")
	}
	if loggerAdapter == nil {
		t.Fatal("loggerAdapter is nil")
	}
}

func TestNewMetricsAdapter_NilMetrics(t *testing.T) {
	// Test behavior with nil metrics (should not panic during creation)
	adapter := NewMetricsAdapter(nil)
	if adapter == nil {
		t.Fatal("NewMetricsAdapter() should not return nil even with nil input")
	}

	// Note: Calling methods on this adapter would panic, but creation should succeed
}

func TestMetricsAdapter_PrometheusIntegration(t *testing.T) {
	// Test that metrics can be registered with Prometheus
	registry := prometheus.NewRegistry()
	m := metrics.New("test")

	// Register metrics
	if err := registry.Register(m.UniFiAPIDuration); err != nil {
		// Ignore already registered error
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			t.Errorf("Failed to register UniFiAPIDuration: %v", err)
		}
	}

	adapter := NewMetricsAdapter(m)

	// Use the adapter
	adapter.RecordUniFiAPICall("test", 100*time.Millisecond, 512, nil)

	// Verify metrics can be gathered
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Just verify we can gather metrics (actual values are tested in metrics package)
	if len(metricFamilies) == 0 {
		t.Log("Warning: No metric families gathered (this is okay if metrics were already registered)")
	}
}
