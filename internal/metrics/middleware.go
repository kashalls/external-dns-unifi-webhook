package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/httpx"
)

// RouteLabel reduces a request to a bounded metric label: the matched route's
// template path, or "other" for any request that did not match a registered
// route (a 404). Using r.Pattern (set by http.ServeMux during routing) rather
// than r.URL.Path keeps the label cardinality bounded by the number of
// registered routes — even a 404 flood, or a subtree pattern like
// "GET /debug/pprof/" whose request paths vary, collapses to a single series.
//
// r.Pattern is only populated after the request has been routed, so callers
// must invoke this AFTER next.ServeHTTP.
func RouteLabel(r *http.Request) string {
	if r.Pattern == "" {
		return "other"
	}
	// r.Pattern is "[METHOD ][HOST]/path"; the path begins at the first slash.
	if i := strings.IndexByte(r.Pattern, '/'); i >= 0 {
		return r.Pattern[i:]
	}

	return "other"
}

// HTTPMetricsMiddleware records request count, duration, in-flight count and
// response size for every wrapped HTTP handler.
func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := Get()

		m.HTTPRequestsInFlight.WithLabelValues(ProviderName).Inc()
		defer m.HTTPRequestsInFlight.WithLabelValues(ProviderName).Dec()

		rec := httpx.NewResponseRecorder(w)
		start := time.Now()

		next.ServeHTTP(rec, r)

		m.RecordHTTPRequest(r.Method, RouteLabel(r), rec.Status(), time.Since(start), rec.Written())
	})
}
