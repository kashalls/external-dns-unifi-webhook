package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/home-operations/external-dns-unifi-webhook/internal/config"
	"github.com/home-operations/external-dns-unifi-webhook/internal/dnsprovider"
	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	"github.com/home-operations/external-dns-unifi-webhook/internal/server"
	"github.com/home-operations/external-dns-unifi-webhook/internal/webhook"
)

const banner = `
external-dns-provider-unifi
version: %s (%s)

`

var (
	Version = "local"
	Gitsha  = "?"
)

func main() {
	initLogger()
	slog.Info(fmt.Sprintf(banner, Version, Gitsha))

	metrics.New(Version)

	cfg, err := config.Init()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	provider, err := dnsprovider.Init(&cfg)
	if err != nil {
		slog.Error("failed to initialize provider", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT,
	)
	defer stop()

	// /readyz delegates to provider.Records — failures mean the upstream
	// UniFi controller is unreachable or credentials are invalid, both of
	// which should keep the pod out of rotation.
	probe := func(ctx context.Context) error {
		_, err := provider.Records(ctx)

		return err
	}

	if err := server.Run(ctx, &cfg, webhook.New(provider), probe); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func initLogger() {
	var level slog.Level
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{AddSource: true, Level: level}

	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
