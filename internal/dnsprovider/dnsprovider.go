package dnsprovider

import (
	"fmt"
	"log/slog"

	"github.com/caarlos0/env/v11"
	"github.com/home-operations/external-dns-unifi-webhook/internal/config"
	"github.com/home-operations/external-dns-unifi-webhook/internal/unifi"
	"sigs.k8s.io/external-dns/provider"
)

//nolint:ireturn // Must return provider.Provider interface per external-dns contract
func Init(_ *config.Config) (provider.Provider, error) {
	unifiConfig := unifi.Config{}
	if err := env.Parse(&unifiConfig); err != nil {
		return nil, fmt.Errorf("reading unifi configuration: %w", err)
	}

	slog.Info("creating unifi provider",
		"host", unifiConfig.Host, "site", unifiConfig.Site)

	p, err := unifi.NewUnifiProvider(&unifiConfig)
	if err != nil {
		return nil, fmt.Errorf("creating UniFi provider: %w", err)
	}

	return p, nil
}
