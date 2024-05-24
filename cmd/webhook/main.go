package main

import (
	"fmt"

	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/configuration"
	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/dnsprovider"
	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/logging"
	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/server"
	"github.com/kashalls/external-dns-provider-unifi/pkg/webhook"
	log "github.com/sirupsen/logrus"
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

	logging.Init()

	config := configuration.Init()
	provider, err := dnsprovider.Init(config)
	if err != nil {
		log.Fatalf("failed to initialize provider: %v", err)
	}

	srv := server.Init(config, webhook.New(provider))
	server.ShutdownGracefully(srv)
}
