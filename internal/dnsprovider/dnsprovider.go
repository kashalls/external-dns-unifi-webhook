package dnsprovider

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/home-operations/external-dns-unifi-webhook/internal/config"
	"github.com/home-operations/external-dns-unifi-webhook/internal/unifi"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider"
)

type UnifiProviderFactory func(baseProvider *provider.BaseProvider, unifiConfig *unifi.Config) provider.Provider

//nolint:ireturn // Must return provider.Provider interface per external-dns contract
func Init(cfg *config.Config) (provider.Provider, error) {
	var domainFilter endpoint.DomainFilter
	createMsg := "creating unifi provider with "

	if cfg.RegexDomainFilter != "" {
		createMsg += fmt.Sprintf("regexp domain filter: '%s', ", cfg.RegexDomainFilter)
		if cfg.RegexDomainExclusion != "" {
			createMsg += fmt.Sprintf("with exclusion: '%s', ", cfg.RegexDomainExclusion)
		}
		domainFilter = *endpoint.NewRegexDomainFilter(
			regexp.MustCompile(cfg.RegexDomainFilter),
			regexp.MustCompile(cfg.RegexDomainExclusion),
		)
	} else {
		if len(cfg.DomainFilter) > 0 {
			createMsg += fmt.Sprintf("domain filter: '%s', ", strings.Join(cfg.DomainFilter, ","))
		}
		if len(cfg.ExcludeDomains) > 0 {
			createMsg += fmt.Sprintf("exclude domain filter: '%s', ", strings.Join(cfg.ExcludeDomains, ","))
		}
		domainFilter = *endpoint.NewDomainFilterWithExclusions(cfg.DomainFilter, cfg.ExcludeDomains)
	}

	createMsg = strings.TrimSuffix(createMsg, ", ")
	if strings.HasSuffix(createMsg, "with ") {
		createMsg += "no kind of domain filters"
	}
	slog.Info(createMsg)

	unifiConfig := unifi.Config{}
	if err := env.Parse(&unifiConfig); err != nil {
		return nil, fmt.Errorf("reading unifi configuration: %w", err)
	}

	p, err := unifi.NewUnifiProvider(domainFilter, &unifiConfig)
	if err != nil {
		return nil, fmt.Errorf("creating UniFi provider: %w", err)
	}

	return p, nil
}
