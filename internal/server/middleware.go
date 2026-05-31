package server

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
)

// statusRecorder wraps http.ResponseWriter to capture the status code written
// by the handler. Defaults to 200 since Go writes that implicitly if no
// WriteHeader call is made before the first Write.
type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

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
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if rv := recover(); rv != nil {
				metrics.Get().PanicsTotal.WithLabelValues(metrics.ProviderName, r.URL.Path).Inc()
				slog.Error("panic in HTTP handler",
					"panic", rv,
					"req_method", r.Method,
					"req_path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				// Best-effort response. If the handler already wrote anything,
				// this header call is a no-op and the client just sees a
				// truncated response, but at least the goroutine doesn't
				// crash the process.
				rec.WriteHeader(http.StatusInternalServerError)
			}

			level := slog.LevelInfo
			switch {
			case rec.status >= http.StatusInternalServerError:
				level = slog.LevelError
			case rec.status >= http.StatusBadRequest:
				level = slog.LevelWarn
			}

			slog.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
			)
		}()

		next.ServeHTTP(rec, r)
	})
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

// IsBodyTooLarge reports whether err comes from a MaxBytesReader limit
// having been exceeded — used by handlers to map decode failures to 413.
func IsBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError

	return errors.As(err, &maxErr)
}
