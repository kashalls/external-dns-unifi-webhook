package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/httpx"
	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
)

// recoveryAndAccessLog wraps next with two cross-cutting concerns:
//
//  1. Panic recovery — any panic in a downstream handler is caught, logged
//     with its stack trace, recorded as a metric, and turned into a 500.
//     Without this the goroutine panics and the client request hangs until
//     the server times out.
//  2. Per-request access log — emits a single slog line per request with
//     method/path/status/duration. Level mirrors the status class.
func recoveryAndAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := httpx.NewResponseRecorder(w)

		defer func() {
			if rv := recover(); rv != nil {
				metrics.Get().PanicsTotal.WithLabelValues(metrics.ProviderName, metrics.RouteLabel(r)).Inc()
				slog.Error("panic in HTTP handler",
					"panic", rv,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				// Best-effort response. If the handler already wrote anything,
				// this header call is a no-op and the client just sees a
				// truncated response, but at least the goroutine doesn't
				// crash the process.
				rec.WriteHeader(http.StatusInternalServerError)
			}

			status := rec.Status()
			level := slog.LevelInfo
			switch {
			case status >= http.StatusInternalServerError:
				level = slog.LevelError
			case status >= http.StatusBadRequest:
				level = slog.LevelWarn
			case isProbePath(r.URL.Path):
				// Successful liveness, readiness, and metrics scrapes fire
				// every few seconds; logging each one at INFO drowns out
				// anything actually interesting. Failures still surface as
				// WARN/ERROR by the cases above.
				level = slog.LevelDebug
			}

			slog.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		}()

		next.ServeHTTP(rec, r)
	})
}

// isProbePath reports whether path is a routine probe / scrape endpoint that
// fires on a fixed schedule (every few seconds in production). Successful
// responses on these paths are demoted to DEBUG so they don't drown out the
// actual external-dns request flow in the logs.
func isProbePath(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/metrics":
		return true
	}

	return false
}

// limitBody wraps next so that each request body is capped at maxBytes. Any
// attempt to read past that limit returns a *http.MaxBytesError, which the
// handler can translate into a 413.
func limitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
