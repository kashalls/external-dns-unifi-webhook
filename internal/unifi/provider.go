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

// UnifiProvider type for interfacing with UniFi
type UnifiProvider struct {
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

	p := &UnifiProvider{
		client:       c,
		domainFilter: domainFilter,
	}

	return p, nil
}

// Records returns the list of records in the DNS provider.
func (p *UnifiProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.GetEndpoints()
	if err != nil {
		return nil, err
	}

	groups := make(map[string][]DNSRecord)
	for _, r := range records {
		if provider.SupportedRecordType(r.RecordType) {
			groupKey := r.Key + r.RecordType
			groups[groupKey] = append(groups[groupKey], r)
		}
	}

	var endpoints []*endpoint.Endpoint
	for _, records := range groups {
		if len(records) == 0 {
			continue
		}

		targets := make([]string, len(records))
		for i, record := range records {
			targets[i] = record.Value
		}

		if ep := endpoint.NewEndpointWithTTL(
			records[0].Key, records[0].RecordType, endpoint.TTL(records[0].TTL), targets...,
		); ep != nil {
			endpoints = append(endpoints, ep)
		}
	}

	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *UnifiProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
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
func (p *UnifiProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}
