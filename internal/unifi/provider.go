package unifi

import (
	"context"
	"fmt"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// Provider type for interfacing with Adguard
type Provider struct {
	provider.BaseProvider

	client       *Client
	domainFilter endpoint.DomainFilter
}

// newUnifiProvider initializes a new DNSProvider.
func NewUnifiProvider(domainFilter endpoint.DomainFilter, config *Configuration) (provider.Provider, error) {
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
	records, err := p.client.ListRecords()
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, record := range records {
		ep := endpoint.NewEndpointWithTTL(
			record.Key,
			record.RecordType,
			record.TTL,
			record.Value,
		)

		if !p.domainFilter.Match(ep.DNSName) {
			continue
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	for _, ep := range changes.Delete {
		if err := p.client.DeleteEndpoint(ep); err != nil {
			return err
		}
	}

	for _, ep := range changes.UpdateNew {
		if _, err := p.client.UpdateEndpoint(ep); err != nil {
			return err
		}
	}

	for _, ep := range changes.Create {
		if _, err := p.client.CreateEndpoint(ep); err != nil {
			return err
		}
	}

	return nil
}

// GetDomainFilter returns the domain filter for the provider.
func (p *Provider) GetDomainFilter() endpoint.DomainFilter {
	return p.domainFilter
}
