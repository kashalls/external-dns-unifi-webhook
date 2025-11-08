package dnsprovider

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/configuration"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/internal/unifi"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider"
)

type UnifiProviderFactory func(baseProvider *provider.BaseProvider, unifiConfig *unifi.Config) provider.Provider

func Init(config configuration.Config) (provider.Provider, error) {
	var domainFilter endpoint.DomainFilter
	createMsg := "creating unifi provider with "

	if config.RegexDomainFilter != "" {
		createMsg += fmt.Sprintf("regexp domain filter: '%s', ", config.RegexDomainFilter)
		if config.RegexDomainExclusion != "" {
			createMsg += fmt.Sprintf("with exclusion: '%s', ", config.RegexDomainExclusion)
		}
		domainFilter = *endpoint.NewRegexDomainFilter(
			regexp.MustCompile(config.RegexDomainFilter),
			regexp.MustCompile(config.RegexDomainExclusion),
		)
	} else {
		if config.DomainFilter != nil && len(config.DomainFilter) > 0 {
			createMsg += fmt.Sprintf("domain filter: '%s', ", strings.Join(config.DomainFilter, ","))
		}
		if config.ExcludeDomains != nil && len(config.ExcludeDomains) > 0 {
			createMsg += fmt.Sprintf("exclude domain filter: '%s', ", strings.Join(config.ExcludeDomains, ","))
		}
		domainFilter = *endpoint.NewDomainFilterWithExclusions(config.DomainFilter, config.ExcludeDomains)
	}

	createMsg = strings.TrimSuffix(createMsg, ", ")
	if strings.HasSuffix(createMsg, "with ") {
		createMsg += "no kind of domain filters"
	}
	log.Info(createMsg)

	unifiConfig := unifi.Config{}
	if err := env.Parse(&unifiConfig); err != nil {
		return nil, errors.Wrap(err, "reading unifi configuration failed")
	}

	p, err := unifi.NewUnifiProvider(domainFilter, &unifiConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UniFi provider")
	}

	return p, nil
}
