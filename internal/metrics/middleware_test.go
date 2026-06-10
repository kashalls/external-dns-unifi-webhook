package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestRouteLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		want    string
	}{
		{"GET /records", "/records"},
		{"POST /adjustendpoints", "/adjustendpoints"},
		{"GET /", "/"},
		{"GET /debug/pprof/", "/debug/pprof/"}, // subtree pattern: bounded despite varying request paths
		{"/records", "/records"},               // no method prefix
		{"", "other"},                          // unmatched (404)
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()
			if got := RouteLabel(&http.Request{Pattern: tt.pattern}); got != tt.want {
				t.Errorf("RouteLabel(pattern=%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestHTTPMetricsMiddleware_BoundsEndpointLabel is the #224 regression: requests
// to unmatched paths must all collapse to a single endpoint="other" label
// instead of minting a new series per raw path (unbounded cardinality), while a
// matched route keeps its template path.
func TestHTTPMetricsMiddleware_BoundsEndpointLabel(t *testing.T) {
	m := Get()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /records", func(http.ResponseWriter, *http.Request) {})
	h := HTTPMetricsMiddleware(mux)

	count := func(endpoint, code string) float64 {
		var dm dto.Metric
		_ = m.HTTPRequestsTotal.WithLabelValues(ProviderName, http.MethodGet, endpoint, code).Write(&dm)

		return dm.GetCounter().GetValue()
	}

	// Three distinct unmatched paths must all land on endpoint="other".
	before := count("other", "404")
	for _, p := range []string{"/nope-a", "/nope-b", "/nope-c"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}
	if got := count("other", "404") - before; got != 3 {
		t.Errorf(`endpoint="other"{404} delta = %v, want 3 (every unmatched path collapses to one series)`, got)
	}

	// A matched route is recorded under its template path.
	beforeMatched := count("/records", "200")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/records", nil))
	if got := count("/records", "200") - beforeMatched; got != 1 {
		t.Errorf(`endpoint="/records"{200} delta = %v, want 1`, got)
	}
}
