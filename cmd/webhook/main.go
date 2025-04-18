package main

import (
	"fmt"

	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/configuration"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/dnsprovider"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/server"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/webhook"

	"go.uber.org/zap"
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
	fmt.Printf(banner, Version, Gitsha)

	log.Init()

	config := configuration.Init()
	provider, err := dnsprovider.Init(config)
	if err != nil {
		log.Fatal("failed to initialize provider", zap.Error(err))
	}

	main, health := server.Init(config, webhook.New(provider))
	server.ShutdownGracefully(main, health)
}
