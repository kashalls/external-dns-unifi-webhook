package unifi

import (
	"context"
	"fmt"

	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"go.uber.org/zap"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// Provider type for interfacing with UniFi
type Provider struct {
	provider.BaseProvider

	client       *httpClient
	domainFilter endpoint.DomainFilter
}

// NewUnifiProvider initializes a new DNSProvider.
func NewUnifiProvider(domainFilter endpoint.DomainFilter, config *Config) (provider.Provider, error) {
	c, err := newUnifiClient(config)

	if err != nil {
		return nil, fmt.Errorf("failed to create the unifi client: %w", err)
	}

	p := &Provider{
		client:       c,
		domainFilter: domainFilter,
	}

	return p, nil
}

// Records returns the list of records in the DNS provider.
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.GetEndpoints()
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, record := range records {
		ep := &endpoint.Endpoint{
			DNSName:    record.Key,
			RecordType: record.RecordType,
			RecordTTL:  record.TTL,
			Targets:    endpoint.NewTargets(record.Value),
		}

		if !p.domainFilter.Match(ep.DNSName) {
			continue
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	for _, endpoint := range append(changes.UpdateOld, changes.Delete...) {
		log.Debug("deleting endpoint", zap.String("name", endpoint.DNSName), zap.String("type", endpoint.RecordType))

		if err := p.client.DeleteEndpoint(endpoint); err != nil {
			log.Error("failed to delete endpoint", zap.String("name", endpoint.DNSName), zap.String("type", endpoint.RecordType), zap.Error(err))
			return err
		}
	}

	for _, endpoint := range append(changes.Create, changes.UpdateNew...) {
		log.Debug("creating endpoint", zap.String("name", endpoint.DNSName), zap.String("type", endpoint.RecordType))

		if _, err := p.client.CreateEndpoint(endpoint); err != nil {
			log.Error("failed to create endpoint", zap.String("name", endpoint.DNSName), zap.String("type", endpoint.RecordType), zap.Error(err))
			return err
		}
	}

	return nil
}

// GetDomainFilter returns the domain filter for the provider.
func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}
