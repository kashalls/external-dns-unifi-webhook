package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// httpClient is the DNS provider client.
type httpClient struct {
	*Config
	*http.Client

	recordsURL string
}

const (
	unifiRecordPath         = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
	unifiRecordPathExternal = "%s/v2/api/site/%s/static-dns/%s"

	recordTypeA     = "A"
	recordTypeAAAA  = "AAAA"
	recordTypeCNAME = "CNAME"
	recordTypeMX    = "MX"
	recordTypeNS    = "NS"
	recordTypeSRV   = "SRV"
	recordTypeTXT   = "TXT"
)

// newUnifiClient constructs a UniFi API client authenticated by UNIFI_API_KEY.
// Cookie/CSRF authentication is no longer supported.
func newUnifiClient(config *Config) (*httpClient, error) {
	transport, err := newHTTPTransport(config)
	if err != nil {
		return nil, err
	}

	recordsURL := unifiRecordPath
	if config.ExternalController {
		recordsURL = unifiRecordPathExternal
	}

	return &httpClient{
		Config:     config,
		Client:     &http.Client{Transport: transport},
		recordsURL: recordsURL,
	}, nil
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *httpClient) GetEndpoints(ctx context.Context) ([]DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	resp, err := c.doRequest(ctx, http.MethodGet, FormatURL(c.recordsURL, c.Host, c.Site), nil)
	duration := time.Since(start)
	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, fmt.Errorf("fetching DNS records from UniFi: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, NewDataError("read", "get endpoints response body", err)
	}

	var records []DNSRecord
	if err := json.Unmarshal(bodyBytes, &records); err != nil {
		slog.Error("failed to decode response", "error", err)
		m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), err)

		return nil, NewDataError("unmarshal", "DNS records", err)
	}

	m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), nil)
	collapseSRVRecords(records)
	slog.Debug("fetched records", "count", len(records))

	return records, nil
}

// collapseSRVRecords folds the priority/weight/port fields into the Value
// string and zeroes the pointers so callers see the same shape the
// external-dns SRV target convention expects.
func collapseSRVRecords(records []DNSRecord) {
	for i, record := range records {
		if record.RecordType != recordTypeSRV {
			continue
		}
		records[i].Value = fmt.Sprintf("%d %d %d %s",
			*record.Priority, *record.Weight, *record.Port, record.Value)
		records[i].Priority = nil
		records[i].Weight = nil
		records[i].Port = nil
	}
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *httpClient) CreateEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) ([]*DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	if endpoint.RecordType == recordTypeCNAME && len(endpoint.Targets) > 1 {
		m.IgnoredCNAMETargetsTotal.WithLabelValues(metrics.ProviderName).Inc()
		slog.Warn("ignoring additional CNAME targets; only the first target will be used",
			"key", endpoint.DNSName, "ignored_targets", endpoint.Targets[1:])
		endpoint.Targets = endpoint.Targets[:1]
	}

	createdRecords := make([]*DNSRecord, 0, len(endpoint.Targets))
	for _, target := range endpoint.Targets {
		record := prepareDNSRecord(endpoint, target)

		if endpoint.RecordType == recordTypeSRV {
			if err := parseSRVTarget(&record, endpoint.Targets[0]); err != nil {
				m.SRVParsingErrorsTotal.WithLabelValues(metrics.ProviderName).Inc()
				m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

				return nil, err
			}
		}

		createdRecord, err := c.createSingleDNSRecord(ctx, &record)
		if err != nil {
			m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

			return nil, err
		}

		createdRecords = append(createdRecords, createdRecord)
		slog.Debug("created new record", "key", createdRecord.Key, "type", createdRecord.RecordType, "target", createdRecord.Value)
	}

	m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, nil)

	return createdRecords, nil
}

func prepareDNSRecord(endpoint *externaldnsendpoint.Endpoint, target string) DNSRecord {
	return DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      target,
	}
}

func parseSRVTarget(record *DNSRecord, target string) error {
	record.Priority = new(int)
	record.Weight = new(int)
	record.Port = new(int)

	_, err := fmt.Sscanf(target, "%d %d %d %s", record.Priority, record.Weight, record.Port, &record.Value)
	if err != nil {
		return NewDataError("parse", "SRV record target", err)
	}

	return nil
}

func (c *httpClient) createSingleDNSRecord(ctx context.Context, record *DNSRecord) (*DNSRecord, error) {
	jsonBody, err := json.Marshal(record)
	if err != nil {
		return nil, NewDataError("marshal", "DNS record", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, FormatURL(c.recordsURL, c.Host, c.Site), jsonBody)
	if err != nil {
		return nil, fmt.Errorf("creating DNS record: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewDataError("read", "create endpoint response body", err)
	}

	var createdRecord DNSRecord
	if err := json.Unmarshal(bodyBytes, &createdRecord); err != nil {
		return nil, NewDataError("unmarshal", "created DNS record", err)
	}

	return &createdRecord, nil
}

// DeleteEndpoint deletes a DNS record from the UniFi controller.
func (c *httpClient) DeleteEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) error {
	m := metrics.Get()
	start := time.Now()

	records, err := c.GetEndpoints(ctx)
	if err != nil {
		m.RecordUniFiAPICall("delete_endpoint", time.Since(start), 0, err)

		return fmt.Errorf("fetching records before deletion: %w", err)
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key != endpoint.DNSName || record.RecordType != endpoint.RecordType {
			continue
		}

		deleteURL := FormatURL(c.recordsURL, c.Host, c.Site, record.ID)
		resp, err := c.doRequest(ctx, http.MethodDelete, deleteURL, nil)
		if err != nil {
			deleteErrors = append(deleteErrors, err)

			continue
		}
		_ = resp.Body.Close()
		slog.Debug("client successfully removed record", "key", record.Key, "type", record.RecordType, "target", record.Value)
	}

	duration := time.Since(start)
	if len(deleteErrors) > 0 {
		err := fmt.Errorf("failed to delete %d records: %w", len(deleteErrors), errors.Join(deleteErrors...))
		m.RecordUniFiAPICall("delete_endpoint", duration, 0, err)

		return err
	}
	m.RecordUniFiAPICall("delete_endpoint", duration, 0, nil)

	return nil
}
