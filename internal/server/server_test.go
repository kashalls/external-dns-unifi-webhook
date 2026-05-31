package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/config"
)

// capturingHandler records the level of the last log record so tests can
// assert log-level routing without parsing JSON output.
type capturingHandler struct{ out *slog.Level }

func newCapturingHandler(out *slog.Level) slog.Handler            { return &capturingHandler{out: out} }
func (capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h capturingHandler) Handle(_ context.Context, r slog.Record) error {
	*h.out = r.Level

	return nil
}
func (h capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h capturingHandler) WithGroup(string) slog.Handler      { return h }

func TestCachedProbe_CachesResultWithinTTL(t *testing.T) {
	var calls atomic.Int32
	probe := func(_ context.Context) error {
		calls.Add(1)

		return nil
	}
	cached := newCachedProbe(probe, 100*time.Millisecond)

	for range 5 {
		if err := cached.Check(context.Background()); err != nil {
			t.Fatalf("Check: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected probe called once within TTL, got %d", got)
	}
}

func TestCachedProbe_RefreshesAfterTTL(t *testing.T) {
	var calls atomic.Int32
	probe := func(_ context.Context) error {
		calls.Add(1)

		return nil
	}
	cached := newCachedProbe(probe, 10*time.Millisecond)

	_ = cached.Check(context.Background())
	time.Sleep(20 * time.Millisecond)
	_ = cached.Check(context.Background())

	if got := calls.Load(); got != 2 {
		t.Errorf("expected probe called twice across TTL boundary, got %d", got)
	}
}

func TestCachedProbe_PropagatesError(t *testing.T) {
	wantErr := errors.New("upstream down")
	cached := newCachedProbe(func(_ context.Context) error { return wantErr }, time.Minute)

	if got := cached.Check(context.Background()); !errors.Is(got, wantErr) {
		t.Errorf("Check error = %v, want %v", got, wantErr)
	}
}

func TestReadyzHandler_ReturnsServiceUnavailableWhenProbeFails(t *testing.T) {
	handler := readyzHandler(func(_ context.Context) error { return errors.New("nope") }, time.Minute)

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nope") {
		t.Errorf("body should include cause; got %q", rec.Body.String())
	}
}

func TestReadyzHandler_OK(t *testing.T) {
	handler := readyzHandler(NoopProbe, time.Minute)

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestBuildMainHandler_MountsHealthEndpoints locks in that /healthz and
// /readyz are reachable on the webhook port, not just the health port.
// Operators rely on this so Kubernetes probes can target the webhook port
// directly (most Helm charts expose only one container port per sidecar).
func TestBuildMainHandler_MountsHealthEndpoints(t *testing.T) {
	cfg := &config.Config{ServerMaxBodyBytes: 1 << 20}
	readyz := readyzHandler(NoopProbe, time.Minute)
	h := buildMainHandler(cfg, nil, readyz)

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s on main handler = %d, want 200", path, rec.Code)
		}
	}
}

// TestAccessLog_DemotesProbePaths verifies that successful /healthz, /readyz,
// and /metrics scrapes log at DEBUG instead of INFO so they don't drown out
// the actual reconciliation traffic. Failures on those paths still surface
// at WARN/ERROR — only 2xx/3xx are quieted.
func TestAccessLog_DemotesProbePaths(t *testing.T) {
	tests := []struct {
		path   string
		status int
		want   slog.Level
	}{
		{"/healthz", http.StatusOK, slog.LevelDebug},
		{"/readyz", http.StatusOK, slog.LevelDebug},
		{"/metrics", http.StatusOK, slog.LevelDebug},
		{"/records", http.StatusOK, slog.LevelInfo},
		{"/healthz", http.StatusServiceUnavailable, slog.LevelError},
		{"/readyz", http.StatusBadRequest, slog.LevelWarn},
	}

	for _, tt := range tests {
		t.Run(tt.path+"-"+strings.TrimPrefix(http.StatusText(tt.status), ""), func(t *testing.T) {
			var captured slog.Level
			handler := newCapturingHandler(&captured)
			defer slog.SetDefault(slog.Default())
			slog.SetDefault(slog.New(handler))

			h := recoveryAndAccessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if captured != tt.want {
				t.Errorf("path=%s status=%d level = %v, want %v", tt.path, tt.status, captured, tt.want)
			}
		})
	}
}

func TestRecoveryAndAccessLog_CatchesPanic(t *testing.T) {
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	recoveryAndAccessLog(panicker).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rec.Code)
	}
}

func TestLimitBody_TrimsOversizedRequests(t *testing.T) {
	// Inner handler reads body and reports whether the limit error fired.
	var sawLimit atomic.Bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			sawLimit.Store(true)
			w.WriteHeader(http.StatusRequestEntityTooLarge)

			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := limitBody(10)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789ABCDEF"))
	handler.ServeHTTP(rec, req)

	if !sawLimit.Load() {
		t.Error("expected MaxBytesReader to trip; inner handler did not see *http.MaxBytesError")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}
