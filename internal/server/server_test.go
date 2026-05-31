package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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
		if IsBodyTooLarge(err) {
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
