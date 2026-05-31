package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/config"
	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	"github.com/home-operations/external-dns-unifi-webhook/internal/webhook"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
)

const shutdownTimeout = 30 * time.Second

// Run starts the webhook and health HTTP servers and blocks until ctx is done
// or either server fails. probe is invoked by /readyz with a TTL-cached,
// singleflight-deduped wrapper — pass server.NoopProbe to skip the check.
// It returns the first error encountered (other than the expected context
// cancellation that triggers a graceful shutdown).
func Run(ctx context.Context, cfg *config.Config, whk *webhook.Webhook, probe ProbeFunc) error {
	if probe == nil {
		probe = noopProbe
	}

	mainSrv := newServer(
		fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		buildMainHandler(cfg, whk),
		cfg,
	)
	healthSrv := newServer(cfg.HealthServerAddr, buildHealthHandler(cfg, probe), cfg)

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error { return serve(ctx, mainSrv, "webhook") })
	group.Go(func() error { return serve(ctx, healthSrv, "health") })

	return group.Wait()
}

// NoopProbe is exported so callers without a real upstream check can still
// satisfy the Run signature.
var NoopProbe ProbeFunc = noopProbe

func buildMainHandler(cfg *config.Config, whk *webhook.Webhook) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", whk.Negotiate)
	mux.HandleFunc("GET /records", whk.Records)
	mux.HandleFunc("POST /records", whk.ApplyChanges)
	mux.HandleFunc("POST /adjustendpoints", whk.AdjustEndpoints)

	// Middleware order (outermost first): recovery+log -> metrics -> body limit -> mux.
	// recovery must be outermost so it catches panics in any inner layer, and
	// the body limit goes innermost so the wrapped reader is the one the
	// handler actually decodes.
	var h http.Handler = mux
	h = limitBody(cfg.ServerMaxBodyBytes)(h)
	h = metrics.HTTPMetricsMiddleware(h)
	h = recoveryAndAccessLog(h)

	return h
}

func buildHealthHandler(cfg *config.Config, probe ProbeFunc) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", okHandler)
	mux.HandleFunc("GET /readyz", readyzHandler(probe, cfg.ReadinessCacheTTL))

	if cfg.PprofEnabled {
		slog.Warn("pprof endpoints enabled on health server — disable in production unless investigating")
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}

	return recoveryAndAccessLog(mux)
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		slog.Error("failed to write health response", "error", err)
	}
}

func readyzHandler(probe ProbeFunc, ttl time.Duration) http.HandlerFunc {
	cached := newCachedProbe(probe, ttl)

	return func(w http.ResponseWriter, r *http.Request) {
		if err := cached.Check(r.Context()); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, "not ready: %v\n", err)

			return
		}
		okHandler(w, r)
	}
}

func newServer(addr string, handler http.Handler, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       cfg.ServerReadTimeout,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
		MaxHeaderBytes:    cfg.ServerMaxHeaderBytes,
	}
}

// serve runs srv.ListenAndServe and shuts it down when ctx is cancelled. A
// clean shutdown (or ErrServerClosed) returns nil; any other error propagates.
func serve(ctx context.Context, srv *http.Server, name string) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting server", "name", name, "address", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("%s server: %w", name, err)
	case <-ctx.Done():
		slog.Info("shutting down server", "name", name)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("%s server shutdown: %w", name, err)
		}

		return nil
	}
}
