package main

import (
	"fmt"

	"github.com/kashalls/external-dns-unifi-webhook/dnsprovider"
	"github.com/kashalls/external-dns-unifi-webhook/webhook"
	"github.com/kashalls/external-dns-unifi-webhook/webhook/configuration"
	"github.com/kashalls/external-dns-unifi-webhook/webhook/logging"
	"github.com/kashalls/external-dns-unifi-webhook/webhook/server"
	log "github.com/sirupsen/logrus"
)

const banner = `
:::    ::: ::::    ::: ::::::::::: :::::::::: ::::::::::: 
:+:    :+: :+:+:   :+:     :+:     :+:            :+:     
+:+    +:+ :+:+:+  +:+     +:+     +:+            +:+     
+#+    +:+ +#+ +:+ +#+     +#+     :#::+::#       +#+     
+#+    +#+ +#+  +#+#+#     +#+     +#+            +#+     
#+#    #+# #+#   #+#+#     #+#     #+#            #+#     
 ########  ###    #### ########### ###        ########### 

 external-dns-unifi-webhook
 version: %s

`

var (
	Version = "v0.0.2"
)

func main() {
	fmt.Printf(banner, Version)
	logging.Init()
	config := configuration.Init()

	provider, err := dnsprovider.NewDNSProvider(config.UnifiHost, config.UnifiUser, config.UnifiPass, config.UnifiSkipTLSVerify)

	if err != nil {
		log.Fatalf("Failed to initialize DNS provider: %v", err)
	}
	srv := server.Init(config, webhook.New(provider))
	server.ShutdownGracefully(srv)
}
