package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
