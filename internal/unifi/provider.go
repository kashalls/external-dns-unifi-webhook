package unifi

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const defaultApplyWorkers = 5

// UnifiProvider type for interfacing with UniFi.
//
//nolint:revive // UnifiProvider is the correct name for this provider, renaming would be a breaking change
type UnifiProvider struct {
	provider.BaseProvider

	client       *httpClient
	domainFilter endpoint.DomainFilter
	workers      int
}

// NewUnifiProvider initializes a new DNSProvider.
//
//nolint:ireturn // Must return provider.Provider interface as required by external-dns API
func NewUnifiProvider(domainFilter endpoint.DomainFilter, config *Config) (provider.Provider, error) {
	c, err := newUnifiClient(config)
	if err != nil {
		return nil, fmt.Errorf("creating unifi client: %w", err)
	}

	workers := config.ApplyWorkers
	if workers <= 0 {
		workers = defaultApplyWorkers
	}

	return &UnifiProvider{
		client:       c,
		domainFilter: domainFilter,
		workers:      workers,
	}, nil
}

// Records returns the list of records in the DNS provider.
func (p *UnifiProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	m := metrics.Get()

	records, err := p.client.GetEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching DNS records: %w", err)
	}

	// Group supported records by key+type and tally per-type counts in one pass.
	groups := make(map[string][]DNSRecord)
	counts := make(map[string]int)
	for _, r := range records {
		if !provider.SupportedRecordType(r.RecordType) {
			continue
		}
		groups[r.Key+r.RecordType] = append(groups[r.Key+r.RecordType], r)
		counts[r.RecordType]++
	}

	for recordType, count := range counts {
		m.UpdateRecordsByType(recordType, count)
	}

	endpoints := make([]*endpoint.Endpoint, 0, len(groups))
	for _, group := range groups {
		targets := make([]string, len(group))
		for i, r := range group {
			targets[i] = r.Value
		}

		if ep := endpoint.NewEndpointWithTTL(
			group[0].Key, group[0].RecordType, group[0].TTL, targets...,
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
		slog.Error("failed to get records while applying", "error", err)

		return fmt.Errorf("fetching existing records before applying changes: %w", err)
	}

	m.BatchSize.WithLabelValues(metrics.ProviderName, "create").Observe(float64(len(changes.Create)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "update").Observe(float64(len(changes.UpdateNew)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "delete").Observe(float64(len(changes.Delete)))

	// Index existing CNAMEs by DNSName so conflict detection on create is O(1).
	existingCNAMEs := make(map[string]*endpoint.Endpoint)
	for _, r := range existingRecords {
		if r.RecordType == recordTypeCNAME {
			existingCNAMEs[r.DNSName] = r
		}
	}

	// Deletes (UpdateOld + Delete) and creates (Create + UpdateNew) each run
	// under their own errgroup with SetLimit so we get bounded concurrency
	// without a hand-rolled semaphore. external-dns already sequences delete
	// before create at the plan level, so we keep the two phases ordered but
	// parallelise within each phase.
	deleteEPs := slices.Concat(changes.UpdateOld, changes.Delete)
	if err := p.runBounded(ctx, deleteEPs, func(ctx context.Context, ep *endpoint.Endpoint) error {
		if err := p.client.DeleteEndpoint(ctx, ep); err != nil {
			slog.Error("failed to delete endpoint", "data", ep, "error", err)

			return fmt.Errorf("deleting endpoint %s (%s): %w", ep.DNSName, ep.RecordType, err)
		}
		m.RecordChange("delete", ep.RecordType)

		return nil
	}); err != nil {
		return err
	}

	createEPs := slices.Concat(changes.Create, changes.UpdateNew)
	if err := p.runBounded(ctx, createEPs, func(ctx context.Context, ep *endpoint.Endpoint) error {
		if ep.RecordType == recordTypeCNAME {
			if existing, ok := existingCNAMEs[ep.DNSName]; ok {
				m.CNAMEConflictsTotal.WithLabelValues(metrics.ProviderName).Inc()
				if err := p.client.DeleteEndpoint(ctx, existing); err != nil {
					slog.Error("failed to delete conflicting CNAME", "data", existing, "error", err)

					return fmt.Errorf("deleting conflicting CNAME %s: %w", existing.DNSName, err)
				}
			}
		}
		if _, err := p.client.CreateEndpoint(ctx, ep); err != nil {
			slog.Error("failed to create endpoint", "data", ep, "error", err)

			return fmt.Errorf("creating endpoint %s (%s): %w", ep.DNSName, ep.RecordType, err)
		}
		m.RecordChange("create", ep.RecordType)

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// runBounded executes fn(ctx, ep) for every entry in eps with at most
// p.workers in flight at any time. The first non-nil error cancels the
// context and bubbles up; remaining workers see ctx.Err() and short-circuit.
func (p *UnifiProvider) runBounded(
	ctx context.Context,
	eps []*endpoint.Endpoint,
	fn func(context.Context, *endpoint.Endpoint) error,
) error {
	if len(eps) == 0 {
		return nil
	}

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(p.workers)

	for _, ep := range eps {
		group.Go(func() error { return fn(gctx, ep) })
	}

	return group.Wait()
}

// GetDomainFilter returns the domain filter for the provider.
//
//nolint:ireturn // Must return endpoint.DomainFilterInterface as required by external-dns API
func (p *UnifiProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &p.domainFilter
}
