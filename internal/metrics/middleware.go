package metrics

import (
	"fmt"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter

	statusCode int
	written    int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	if err != nil {
		return n, fmt.Errorf("writing response: %w", err)
	}

	return n, nil
}

// HTTPMetricsMiddleware records request count, duration, in-flight count and
// response size for every wrapped HTTP handler.
func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := Get()

		m.HTTPRequestsInFlight.WithLabelValues(ProviderName).Inc()
		defer m.HTTPRequestsInFlight.WithLabelValues(ProviderName).Dec()

		rw := newResponseWriter(w)
		start := time.Now()

		next.ServeHTTP(rw, r)

		m.RecordHTTPRequest(r.Method, r.URL.Path, rw.statusCode, time.Since(start), rw.written)
	})
}
