package unifi

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"go.uber.org/zap"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// UnifiProvider type for interfacing with UniFi.
type UnifiProvider struct {
	provider.BaseProvider

	client       *httpClient
	domainFilter endpoint.DomainFilter
}

// NewUnifiProvider initializes a new DNSProvider.
func NewUnifiProvider(domainFilter endpoint.DomainFilter, config *Config) (provider.Provider, error) {
	c, err := newUnifiClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the unifi client")
	}

	p := &UnifiProvider{
		client:       c,
		domainFilter: domainFilter,
	}

	return p, nil
}

// Records returns the list of records in the DNS provider.
func (p *UnifiProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	m := metrics.Get()

	records, err := p.client.GetEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch DNS records")
	}

	// Count records by type for metrics
	recordsByType := make(map[string]int)
	for _, r := range records {
		if provider.SupportedRecordType(r.RecordType) {
			recordsByType[r.RecordType]++
		}
	}

	// Update metrics for each record type
	for recordType, count := range recordsByType {
		m.UpdateRecordsByType(recordType, count)
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
			records[0].Key, records[0].RecordType, records[0].TTL, targets...,
		); ep != nil {
			endpoints = append(endpoints, ep)
		}
	}

	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *UnifiProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	m := metrics.Get()

	existingRecords, err := p.Records(ctx)
	if err != nil {
		log.Error("failed to get records while applying", zap.Error(err))

		return errors.Wrap(err, "failed to get existing records before applying changes")
	}

	// Record batch sizes
	m.BatchSize.WithLabelValues(metrics.ProviderName, "create").Observe(float64(len(changes.Create)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "update").Observe(float64(len(changes.UpdateNew)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "delete").Observe(float64(len(changes.Delete)))

	// Process deletions and updates (delete old)
	for _, endpoint := range append(changes.UpdateOld, changes.Delete...) {
		err := p.client.DeleteEndpoint(endpoint)
		if err != nil {
			log.Error("failed to delete endpoint", zap.Any("data", endpoint), zap.Error(err))

			return errors.Wrapf(err, "failed to delete endpoint %s (%s)", endpoint.DNSName, endpoint.RecordType)
		}
		m.RecordChange("delete", endpoint.RecordType)
	}

	// Process creates and updates (create new)
	for _, endpoint := range append(changes.Create, changes.UpdateNew...) {
		operation := "create"
		// Check for CNAME conflicts
		if endpoint.RecordType == "CNAME" {
			for _, record := range existingRecords {
				if record.RecordType != "CNAME" {
					continue
				}

				if record.DNSName != endpoint.DNSName {
					continue
				}

				m.CNAMEConflictsTotal.WithLabelValues(metrics.ProviderName).Inc()
				err := p.client.DeleteEndpoint(record)
				if err != nil {
					log.Error("failed to delete conflicting CNAME", zap.Any("data", record), zap.Error(err))

					return errors.Wrapf(err, "failed to delete conflicting CNAME %s", record.DNSName)
				}
			}
		}
		if _, err := p.client.CreateEndpoint(endpoint); err != nil {
			log.Error("failed to create endpoint", zap.Any("data", endpoint), zap.Error(err))

			return errors.Wrapf(err, "failed to create endpoint %s (%s)", endpoint.DNSName, endpoint.RecordType)
		}
		m.RecordChange(operation, endpoint.RecordType)
	}

	return nil
}

// GetDomainFilter returns the domain filter for the provider.
func (p *UnifiProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &p.domainFilter
}
