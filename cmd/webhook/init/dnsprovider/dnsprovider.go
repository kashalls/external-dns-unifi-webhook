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
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider"
)

type UnifiProviderFactory func(baseProvider *provider.BaseProvider, unifiConfig *unifi.Config) provider.Provider

//nolint:ireturn,funlen // Must return provider.Provider interface; complexity comes from dependency injection setup
func Init(config *configuration.Config) (provider.Provider, error) {
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
		if len(config.DomainFilter) > 0 {
			createMsg += fmt.Sprintf("domain filter: '%s', ", strings.Join(config.DomainFilter, ","))
		}
		if len(config.ExcludeDomains) > 0 {
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
	err := env.Parse(&unifiConfig)
	if err != nil {
		return nil, errors.Wrap(err, "reading unifi configuration failed")
	}

	// Create adapters for metrics and logger
	metricsAdapter := unifi.NewMetricsAdapter(metrics.Get())
	loggerAdapter := unifi.NewLoggerAdapter()

	// Create HTTP transport
	transport, err := unifi.NewHTTPTransport(&unifiConfig, metricsAdapter, loggerAdapter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP transport")
	}

	// Get ClientURLs from transport (using type assertion for internal access)
	var clientURLs *unifi.ClientURLs
	if ht, ok := transport.(interface{ GetClientURLs() *unifi.ClientURLs }); ok {
		clientURLs = ht.GetClientURLs()
	} else {
		return nil, errors.New("transport does not expose ClientURLs")
	}

	// Create record transformer
	transformer := unifi.NewRecordTransformer()

	// Create UniFi API client with all dependencies
	api := unifi.NewUnifiAPIClient(
		transport,
		transformer,
		metricsAdapter,
		loggerAdapter,
		&unifiConfig,
		clientURLs,
	)

	// Create provider with injected dependencies
	p, err := unifi.NewUnifiProvider(api, domainFilter, metricsAdapter, loggerAdapter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create UniFi provider")
	}

	return p, nil
}
