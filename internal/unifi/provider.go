package unifi

import (
	"context"

	"github.com/cockroachdb/errors"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// UnifiProvider type for interfacing with UniFi.
//
//nolint:revive // UnifiProvider is the correct name for this provider, renaming would be a breaking change
type UnifiProvider struct {
	provider.BaseProvider

	api          UnifiAPI
	domainFilter endpoint.DomainFilter
	metrics      MetricsRecorder
	logger       Logger
}

// NewUnifiProvider initializes a new DNSProvider with dependency injection.
//
//nolint:ireturn // Must return provider.Provider interface as required by external-dns API
func NewUnifiProvider(
	api UnifiAPI,
	domainFilter endpoint.DomainFilter,
	metricsRecorder MetricsRecorder,
	logger Logger,
) (provider.Provider, error) {
	unifiProvider := &UnifiProvider{
		api:          api,
		domainFilter: domainFilter,
		metrics:      metricsRecorder,
		logger:       logger,
	}

	return unifiProvider, nil
}

// NewUnifiProviderFromConfig initializes a new DNSProvider from config - DEPRECATED.
// This function is kept for backward compatibility. Use NewUnifiProvider instead.
//
//nolint:ireturn // Must return provider.Provider interface as required by external-dns API
func NewUnifiProviderFromConfig(domainFilter endpoint.DomainFilter, config *Config) (provider.Provider, error) {
	api, err := newUnifiClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the unifi client")
	}

	metricsAdapter := NewMetricsAdapter(nil)
	loggerAdapter := NewLoggerAdapter()

	return NewUnifiProvider(api, domainFilter, metricsAdapter, loggerAdapter)
}

// Records returns the list of records in the DNS provider.
func (p *UnifiProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.api.GetEndpoints(ctx)
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
		p.metrics.UpdateRecordsByType(recordType, count)
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
	existingRecords, err := p.Records(ctx)
	if err != nil {
		p.logger.Error("failed to get records while applying", "error", err)

		return errors.Wrap(err, "failed to get existing records before applying changes")
	}

	// Record batch sizes
	p.metrics.BatchSize().WithLabelValues("unifi", "create").Observe(float64(len(changes.Create)))
	p.metrics.BatchSize().WithLabelValues("unifi", "update").Observe(float64(len(changes.UpdateNew)))
	p.metrics.BatchSize().WithLabelValues("unifi", "delete").Observe(float64(len(changes.Delete)))

	// Process deletions and updates (delete old)
	for _, endpoint := range append(changes.UpdateOld, changes.Delete...) {
		err := p.api.DeleteEndpoint(ctx, endpoint)
		if err != nil {
			p.logger.Error("failed to delete endpoint", "data", endpoint, "error", err)

			return errors.Wrapf(err, "failed to delete endpoint %s (%s)", endpoint.DNSName, endpoint.RecordType)
		}
		p.metrics.RecordChange("delete", endpoint.RecordType)
	}

	// Process creates and updates (create new)
	for _, endpoint := range append(changes.Create, changes.UpdateNew...) {
		operation := "create"
		// Check for CNAME conflicts
		if endpoint.RecordType == recordTypeCNAME {
			for _, record := range existingRecords {
				if record.RecordType != recordTypeCNAME {
					continue
				}

				if record.DNSName != endpoint.DNSName {
					continue
				}

				p.metrics.CNAMEConflictsTotal().WithLabelValues("unifi").Inc()
				err := p.api.DeleteEndpoint(ctx, record)
				if err != nil {
					p.logger.Error("failed to delete conflicting CNAME", "data", record, "error", err)

					return errors.Wrapf(err, "failed to delete conflicting CNAME %s", record.DNSName)
				}
			}
		}
		_, err := p.api.CreateEndpoint(ctx, endpoint)
		if err != nil {
			p.logger.Error("failed to create endpoint", "data", endpoint, "error", err)

			return errors.Wrapf(err, "failed to create endpoint %s (%s)", endpoint.DNSName, endpoint.RecordType)
		}
		p.metrics.RecordChange(operation, endpoint.RecordType)
	}

	return nil
}

// GetDomainFilter returns the domain filter for the provider.
//
//nolint:ireturn // Must return endpoint.DomainFilterInterface as required by external-dns API
func (p *UnifiProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &p.domainFilter
}
