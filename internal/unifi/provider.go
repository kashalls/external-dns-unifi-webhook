package unifi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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

	client  *httpClient
	workers int
}

// NewUnifiProvider initializes a new DNSProvider. The --domain-filter CLI flag
// is applied at the controller level via the registry; UniFi has no zones to
// further constrain against, so we inherit BaseProvider.GetDomainFilter
// (returns an empty filter — matches everything). See the GetDomainFilter
// contract in sigs.k8s.io/external-dns/docs/contributing/sources-and-providers.md.
//
//nolint:ireturn // Must return provider.Provider interface as required by external-dns API
func NewUnifiProvider(config *Config) (provider.Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid unifi configuration: %w", err)
	}

	c, err := newUnifiClient(config)
	if err != nil {
		return nil, fmt.Errorf("creating unifi client: %w", err)
	}

	return &UnifiProvider{
		client:  c,
		workers: cmp.Or(config.ApplyWorkers, defaultApplyWorkers),
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
//
// The full record set is fetched once at the top and indexed by Key+RecordType
// so per-endpoint deletes and CNAME-conflict cleanup can resolve target
// record IDs locally instead of re-fetching the list per call (which used to
// produce 1+N paginated round trips per reconcile). DeleteRecord tolerates
// 404 so the snapshot index is safe even if a record disappears between
// snapshot and delete.
func (p *UnifiProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	m := metrics.Get()

	rawRecords, err := p.client.GetEndpoints(ctx)
	if err != nil {
		return fmt.Errorf("fetching existing records before applying changes: %w", err)
	}

	m.BatchSize.WithLabelValues(metrics.ProviderName, "create").Observe(float64(len(changes.Create)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "update").Observe(float64(len(changes.UpdateNew)))
	m.BatchSize.WithLabelValues(metrics.ProviderName, "delete").Observe(float64(len(changes.Delete)))

	byKeyType := indexRecordIDs(rawRecords)

	// Deletes (UpdateOld + Delete) and creates (Create + UpdateNew) each run
	// under their own errgroup with SetLimit so we get bounded concurrency
	// without a hand-rolled semaphore. external-dns already sequences delete
	// before create at the plan level, so we keep the two phases ordered but
	// parallelise within each phase.
	deleteEPs := slices.Concat(changes.UpdateOld, changes.Delete)
	if err := p.runBounded(ctx, deleteEPs, func(ctx context.Context, ep *endpoint.Endpoint) error {
		if err := p.deleteByIDs(ctx, byKeyType[ep.DNSName+ep.RecordType]); err != nil {
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
			if ids := byKeyType[ep.DNSName+recordTypeCNAME]; len(ids) > 0 {
				m.CNAMEConflictsTotal.WithLabelValues(metrics.ProviderName).Inc()
				if err := p.deleteByIDs(ctx, ids); err != nil {
					return fmt.Errorf("deleting conflicting CNAME %s: %w", ep.DNSName, err)
				}
			}
		}
		if _, err := p.client.CreateEndpoint(ctx, ep); err != nil {
			return fmt.Errorf("creating endpoint %s (%s): %w", ep.DNSName, ep.RecordType, err)
		}
		m.RecordChange("create", ep.RecordType)

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// indexRecordIDs groups record IDs by Key+RecordType so the delete path can
// resolve "all IDs for endpoint X" in O(1) without re-listing.
func indexRecordIDs(records []DNSRecord) map[string][]string {
	out := make(map[string][]string, len(records))
	for _, r := range records {
		key := r.Key + r.RecordType
		out[key] = append(out[key], r.ID)
	}

	return out
}

// deleteByIDs deletes every record in ids, tolerating per-record failures and
// joining them into a single error if any fail. An empty slice is a no-op.
func (p *UnifiProvider) deleteByIDs(ctx context.Context, ids []string) error {
	var errs []error
	for _, id := range ids {
		if err := p.client.DeleteRecord(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
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
