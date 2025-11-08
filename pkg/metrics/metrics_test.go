package metrics

import (
	"testing"
	"time"

	"github.com/cockroachdb/errors"
)

func TestMetrics(t *testing.T) {
	// Reset singleton for testing
	instance = nil

	t.Run("New", func(t *testing.T) {
		m := New("test-version")
		if m == nil {
			t.Fatal("expected non-nil metrics instance")
		}

		// Check that singleton works
		m2 := Get()
		if m != m2 {
			t.Error("expected Get() to return the same instance")
		}
	})

	t.Run("RecordHTTPRequest", func(t *testing.T) {
		m := Get()
		m.RecordHTTPRequest("GET", "/test", 200, time.Millisecond*100, 1024)

		// Verify metrics were recorded (basic check)
		if m.HTTPRequestsTotal == nil {
			t.Error("HTTPRequestsTotal should not be nil")
		}
	})

	t.Run("RecordUniFiAPICall", func(t *testing.T) {
		m := Get()

		// Test successful call
		m.RecordUniFiAPICall("test_operation", time.Millisecond*50, 512, nil)

		// Test failed call
		testErr := errors.New("test error")
		m.RecordUniFiAPICall("test_operation", time.Millisecond*50, 0, testErr)

		if m.UniFiAPIDuration == nil {
			t.Error("UniFiAPIDuration should not be nil")
		}
	})

	t.Run("UpdateRecordsByType", func(t *testing.T) {
		m := Get()

		m.UpdateRecordsByType("A", 10)
		m.UpdateRecordsByType("AAAA", 5)
		m.UpdateRecordsByType("CNAME", 3)

		if m.RecordsTotal == nil {
			t.Error("RecordsTotal should not be nil")
		}
	})

	t.Run("RecordChange", func(t *testing.T) {
		m := Get()

		m.RecordChange("create", "A")
		m.RecordChange("update", "CNAME")
		m.RecordChange("delete", "TXT")

		if m.ChangesTotal == nil {
			t.Error("ChangesTotal should not be nil")
		}
		if m.ChangesByTypeTotal == nil {
			t.Error("ChangesByTypeTotal should not be nil")
		}
	})
}
