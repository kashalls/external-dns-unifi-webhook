package main

import (
	"fmt"

	"github.com/home-operations/external-dns-unifi-webhook/cmd/webhook/init/configuration"
	"github.com/home-operations/external-dns-unifi-webhook/cmd/webhook/init/dnsprovider"
	"github.com/home-operations/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/home-operations/external-dns-unifi-webhook/cmd/webhook/init/server"
	"github.com/home-operations/external-dns-unifi-webhook/pkg/metrics"
	"github.com/home-operations/external-dns-unifi-webhook/pkg/webhook"
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
	log.Init()
	log.Info(fmt.Sprintf(banner, Version, Gitsha))

	metrics.New(Version)

	config := configuration.Init()
	provider, err := dnsprovider.Init(&config)
	if err != nil {
		log.Fatal("failed to initialize provider", "error", err)
	}

	main, health := server.Init(&config, webhook.New(provider))
	server.ShutdownGracefully(main, health)
}
