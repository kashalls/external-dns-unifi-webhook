package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// newTestMetrics returns a Metrics instance backed by an isolated registry so
// each test starts from a clean slate without touching package globals.
func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()

	return build(promauto.With(prometheus.NewRegistry()), "test-version")
}

func TestNewPopulatesAllVecs(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	if m.Info == nil || m.HTTPRequestsTotal == nil || m.UniFiAPIDuration == nil {
		t.Fatal("expected metric vectors to be populated")
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)
	m.RecordHTTPRequest("GET", "/test", 200, 100*time.Millisecond, 1024)
}

func TestRecordUniFiAPICall(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordUniFiAPICall("test_op", 50*time.Millisecond, 512, nil)
	m.RecordUniFiAPICall("test_op", 50*time.Millisecond, 0, errors.New("boom"))
}

// gaugeValue reads the current value of a single gauge through the dto
// round-trip. We deliberately avoid prometheus testutil here — it would drag a
// test-only transitive dependency in just to read one float.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var metric dto.Metric
	if err := g.Write(&metric); err != nil {
		t.Fatalf("gauge.Write: %v", err)
	}

	return metric.GetGauge().GetValue()
}

func TestRecordOperation(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	consecutive := m.ConsecutiveErrors.WithLabelValues(ProviderName)
	lastSuccess := m.LastSuccessTimestamp.WithLabelValues(ProviderName)

	// Two failing operations accumulate the consecutive-error gauge and leave
	// the last-success timestamp untouched.
	m.RecordOperation(errors.New("boom"))
	m.RecordOperation(errors.New("boom"))
	if got := gaugeValue(t, consecutive); got != 2 {
		t.Errorf("consecutive_errors after 2 failures = %v, want 2", got)
	}
	if got := gaugeValue(t, lastSuccess); got != 0 {
		t.Errorf("last_success_timestamp should stay 0 while failing, got %v", got)
	}

	// A success resets the consecutive-error gauge and stamps last-success.
	m.RecordOperation(nil)
	if got := gaugeValue(t, consecutive); got != 0 {
		t.Errorf("consecutive_errors after success = %v, want 0", got)
	}
	if got := gaugeValue(t, lastSuccess); got <= 0 {
		t.Errorf("last_success_timestamp after success = %v, want > 0", got)
	}
}

func TestUpdateRecordsByType(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.UpdateRecordsByType("A", 10)
	m.UpdateRecordsByType("AAAA", 5)
	m.UpdateRecordsByType("CNAME", 3)
}

func TestRecordChange(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordChange("create", "A")
	m.RecordChange("update", "CNAME")
	m.RecordChange("delete", "TXT")
}

func TestSingletonReturnsSameInstance(t *testing.T) {
	// Cannot run in parallel — exercises the package-wide singleton.
	a := Get()
	b := Get()
	if a != b {
		t.Errorf("Get() returned different instances")
	}
}
