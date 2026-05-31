package metrics

import (
	"net/http"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/httpx"
)

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

		m.RecordHTTPRequest(r.Method, r.URL.Path, rec.Status(), time.Since(start), rec.Written())
	})
}
