package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/configuration"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/webhook"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.uber.org/zap"
)

const (
	shutdownTimeout = 30 * time.Second
)

// HealthCheckHandler returns the status of the service.
func HealthCheckHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		log.Error("failed to write health check response", zap.Error(err))
	}
}

// ReadinessHandler returns whether the service is ready to accept requests.
func ReadinessHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		log.Error("failed to write readiness response", zap.Error(err))
	}
}

// Init initializes the http server.
func Init(config *configuration.Config, p *webhook.Webhook) (*http.Server, *http.Server) {
	mainRouter := chi.NewRouter()
	mainRouter.Use(metrics.HTTPMetricsMiddleware)
	mainRouter.Get("/", p.Negotiate)
	mainRouter.Get("/records", p.Records)
	mainRouter.Post("/records", p.ApplyChanges)
	mainRouter.Post("/adjustendpoints", p.AdjustEndpoints)

	mainServer := createHTTPServer(fmt.Sprintf("%s:%d", config.ServerHost, config.ServerPort), mainRouter, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Info("starting webhook server", zap.String("address", mainServer.Addr))
		err := mainServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("unable to start webhook server", zap.String("address", mainServer.Addr), zap.Error(err))
		}
	}()

	healthRouter := chi.NewRouter()
	healthRouter.Get("/metrics", promhttp.Handler().ServeHTTP)
	healthRouter.Get("/healthz", HealthCheckHandler)
	healthRouter.Get("/readyz", ReadinessHandler)

	healthServer := createHTTPServer("0.0.0.0:8080", healthRouter, config.ServerReadTimeout, config.ServerWriteTimeout)
	go func() {
		log.Info("starting health server", zap.String("address", healthServer.Addr))
		err := healthServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("unable to start health server", zap.String("address", healthServer.Addr), zap.Error(err))
		}
	}()

	return mainServer, healthServer
}

func createHTTPServer(addr string, hand http.Handler, readTimeout, writeTimeout time.Duration) *http.Server {
	return &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Addr:         addr,
		Handler:      hand,
	}
}

// ShutdownGracefully gracefully shutdown the http server.
func ShutdownGracefully(mainServer, healthServer *http.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigCh

	log.Info("shutting down servers due to received signal", zap.Any("signal", sig))
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := mainServer.Shutdown(ctx)
	if err != nil {
		log.Error("error shutting down main server", zap.Error(err))
	}

	err = healthServer.Shutdown(ctx)
	if err != nil {
		log.Error("error shutting down health server", zap.Error(err))
	}
}
