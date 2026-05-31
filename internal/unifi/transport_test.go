package unifi

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
)

func newRetryClient(t *testing.T, srv *httptest.Server, cfg *Config) *httpClient {
	t.Helper()
	_ = metrics.Get()

	if cfg == nil {
		cfg = &Config{}
	}
	cfg.Host = srv.URL
	cfg.Site = "default"
	cfg.APIKey = "test-key"
	cfg.SkipTLSVerify = true
	if cfg.RetryAttempts == 0 {
		cfg.RetryAttempts = 3
	}
	if cfg.RetryInitialDelay == 0 {
		cfg.RetryInitialDelay = 1 * time.Millisecond
	}
	if cfg.RetryMaxDelay == 0 {
		cfg.RetryMaxDelay = 10 * time.Millisecond
	}

	return testClient(srv, cfg)
}

func TestDoRequest_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)

			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newRetryClient(t, srv, nil)

	resp, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/static-dns/", nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	_ = resp.Body.Close()

	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 attempts (2 retries), got %d", got)
	}
}

func TestDoRequest_NoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad"}`))
	}))
	defer srv.Close()

	c := newRetryClient(t, srv, nil)

	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/static-dns/", nil)
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !isAPIError(err) {
		t.Errorf("expected APIError, got %T: %v", err, err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("4xx should not retry: got %d calls, want 1", got)
	}
}

func TestDoRequest_HonorsRetryAfterOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1") // 1 second hint
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newRetryClient(t, srv, &Config{
		RetryAttempts:     3,
		RetryInitialDelay: 1 * time.Millisecond,
		RetryMaxDelay:     2 * time.Second, // big enough to let Retry-After through
	})

	start := time.Now()
	resp, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/static-dns/", nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	_ = resp.Body.Close()

	// We asked for 1s; allow a small clock-skew margin.
	if elapsed < 900*time.Millisecond {
		t.Errorf("retry returned too fast (%v); Retry-After was 1s", elapsed)
	}
}

func TestDoRequest_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"down"}`))
	}))
	defer srv.Close()

	c := newRetryClient(t, srv, &Config{RetryAttempts: 2})

	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/static-dns/", nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !isAPIError(err) {
		t.Errorf("expected APIError surfaced from final attempt, got %T: %v", err, err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected exactly RetryAttempts=2 calls, got %d", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"", 0},
		{"3", 3 * time.Second},
		{"   5  ", 5 * time.Second},
		{"-1", 0},
		{"nope", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseRetryAfter(tt.input)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBackoff_ClampsToMax(t *testing.T) {
	c := &httpClient{cfg: &Config{
		RetryInitialDelay: 1 * time.Second,
		RetryMaxDelay:     2 * time.Second,
	}}

	// attempt=10 would be 1s << 10 = 1024s if unclamped.
	got := c.backoff(10, 0)
	if got > 2*time.Second {
		t.Errorf("backoff did not clamp: %v > 2s", got)
	}
}

func TestNewHTTPTransport_SkipTLSVerify(t *testing.T) {
	tr, err := newHTTPTransport(&Config{SkipTLSVerify: true})
	if err != nil {
		t.Fatalf("newHTTPTransport: %v", err)
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
}

func TestNewHTTPTransport_CACertFromFile(t *testing.T) {
	// httptest's TLS cert is a good stand-in CA for this test — we just need
	// some valid PEM the file loader can parse.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	pemFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(pemFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	tr, err := newHTTPTransport(&Config{CACertPath: pemFile})
	if err != nil {
		t.Fatalf("newHTTPTransport: %v", err)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("CA cert should not coexist with InsecureSkipVerify")
	}
	if tr.TLSClientConfig.RootCAs == nil {
		t.Error("RootCAs not populated")
	}
}

func TestNewHTTPTransport_MissingCAFile(t *testing.T) {
	_, err := newHTTPTransport(&Config{CACertPath: "/nonexistent/ca.pem"})
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

// TestDoRequest_NetworkErrorRetries verifies that connection-refused style
// errors are retried (not just HTTP error responses).
func TestDoRequest_NetworkErrorRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	// Close immediately — any request will hit a closed connection.
	srv.Close()

	c := newRetryClient(t, srv, &Config{RetryAttempts: 2})

	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/static-dns/", nil)
	if err == nil {
		t.Fatal("expected error against closed server")
	}
	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Errorf("expected NetworkError, got %T: %v", err, err)
	}
}
