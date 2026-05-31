package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
	extdnshttp "sigs.k8s.io/external-dns/pkg/http"
)

// httpClient is the DNS provider client.
type httpClient struct {
	*Config
	*http.Client

	siteID string
}

// API paths (Network Integration API, internal connection only).
const (
	pathSites    = "%s/proxy/network/integration/v1/sites"
	pathPolicies = "%s/proxy/network/integration/v1/sites/%s/dns/policies"
	pathPolicy   = "%s/proxy/network/integration/v1/sites/%s/dns/policies/%s"
)

// recordType* are the external-dns / DNSRecord type strings. They map to the
// API's per-type discriminator values in dto.go.
const (
	recordTypeA     = "A"
	recordTypeAAAA  = "AAAA"
	recordTypeCNAME = "CNAME"
	recordTypeMX    = "MX"
	recordTypeSRV   = "SRV"
	recordTypeTXT   = "TXT"
)

// pageLimit is the API's documented maximum page size.
const pageLimit = 200

// siteResolveTimeout caps the startup probe so a broken controller doesn't
// hang the process forever.
const siteResolveTimeout = 15 * time.Second

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// newUnifiClient constructs a UniFi API client and resolves the site name (or
// UUID) supplied via UNIFI_SITE to the API's UUID at startup. The probe also
// validates credentials early so misconfiguration fails fast.
func newUnifiClient(config *Config) (*httpClient, error) {
	transport, err := newHTTPTransport(config)
	if err != nil {
		return nil, err
	}

	// NewInstrumentedClient wraps the transport so every outbound call emits
	// external_dns_http_request_duration_seconds, matching the metric naming
	// the rest of the external-dns ecosystem (and the inbound webhook server)
	// uses. See sigs.k8s.io/external-dns PR #6307.
	c := &httpClient{
		Config: config,
		Client: extdnshttp.NewInstrumentedClient(&http.Client{Transport: transport}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), siteResolveTimeout)
	defer cancel()
	siteID, err := c.resolveSite(ctx, config.Site)
	if err != nil {
		return nil, fmt.Errorf("resolving UNIFI_SITE %q: %w", config.Site, err)
	}
	c.siteID = siteID
	slog.Info("resolved UniFi site", "ref", config.Site, "id", siteID)

	return c, nil
}

// resolveSite turns a user-supplied site name (or UUID) into the API's site
// UUID via GET /v1/sites with a filter. Accepts either the human-friendly
// internalReference (e.g. "default") or a raw UUID — both are common ways
// people identify a site in their environment.
func (c *httpClient) resolveSite(ctx context.Context, ref string) (string, error) {
	field := "internalReference"
	if uuidRegex.MatchString(ref) {
		field = "id"
	}

	filter := fmt.Sprintf("%s.eq('%s')", field, ref)
	u := FormatURL(pathSites, c.Host) + "?filter=" + url.QueryEscape(filter)

	var page sitePage
	if _, err := c.getJSON(ctx, u, "sites", &page); err != nil {
		return "", err
	}

	switch len(page.Data) {
	case 0:
		return "", fmt.Errorf("no site matches %s=%q", field, ref)
	case 1:
		return page.Data[0].ID, nil
	default:
		return "", fmt.Errorf("multiple sites match %s=%q (%d)", field, ref, len(page.Data))
	}
}

// getJSON issues a GET, reads the body fully, and decodes it into dest. The
// scope label is used in error messages ("DNS policies", "sites", ...) so the
// caller doesn't have to repeat it across read/unmarshal sites. Returns the
// number of body bytes read for metric accounting.
func (c *httpClient) getJSON(ctx context.Context, reqURL, scope string, dest any) (int, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, err
	}
	defer extdnshttp.DrainAndClose(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, NewDataError("read", scope+" response body", err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return len(body), NewDataError("unmarshal", scope, err)
	}

	return len(body), nil
}

// GetEndpoints retrieves the full list of DNS records from the UniFi
// controller, walking the paginated DNS Policies endpoint. FORWARD_DOMAIN
// entries and unknown record types are filtered out by the converter so we
// never report them as managed records.
func (c *httpClient) GetEndpoints(ctx context.Context) ([]DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	var (
		records  []DNSRecord
		offset   int
		bodyRead int
	)
	for {
		page, n, err := c.fetchPolicyPage(ctx, offset, pageLimit)
		bodyRead += n
		if err != nil {
			m.RecordUniFiAPICall("get_endpoints", time.Since(start), bodyRead, err)

			return nil, fmt.Errorf("fetching DNS records from UniFi: %w", err)
		}

		for _, env := range page.Data {
			if r, ok := toDNSRecord(env); ok {
				records = append(records, r)
			}
		}

		offset += page.Count
		if page.Count == 0 || offset >= page.TotalCount {
			break
		}
	}

	m.RecordUniFiAPICall("get_endpoints", time.Since(start), bodyRead, nil)
	slog.Debug("fetched records", "count", len(records))

	return records, nil
}

func (c *httpClient) fetchPolicyPage(ctx context.Context, offset, limit int) (dnsPolicyPage, int, error) {
	u := fmt.Sprintf("%s?offset=%d&limit=%d",
		FormatURL(pathPolicies, c.Host, c.siteID), offset, limit)

	var page dnsPolicyPage
	n, err := c.getJSON(ctx, u, "DNS policies", &page)

	return page, n, err
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

	created := make([]*DNSRecord, 0, len(endpoint.Targets))
	for _, target := range endpoint.Targets {
		r := DNSRecord{
			Enabled:    true,
			Key:        endpoint.DNSName,
			RecordType: endpoint.RecordType,
			TTL:        endpoint.RecordTTL,
			Value:      target,
		}

		out, err := c.createOne(ctx, r)
		if err != nil {
			if isSRVConversionError(err) {
				m.SRVParsingErrorsTotal.WithLabelValues(metrics.ProviderName).Inc()
			}
			m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

			return nil, err
		}

		created = append(created, out)
		slog.Debug("created new record", "key", out.Key, "type", out.RecordType, "target", out.Value)
	}

	m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, nil)

	return created, nil
}

// isSRVConversionError reports whether err originated in SRV name or target
// parsing. It guards the SRV-specific metric so unrelated DataErrors (marshal,
// MX parse, unsupported type) don't pollute the count.
func isSRVConversionError(err error) bool {
	var de *DataError
	if !errors.As(err, &de) {
		return false
	}

	return de.DataType == "SRV record target" || de.DataType == "SRV record name"
}

func (c *httpClient) createOne(ctx context.Context, r DNSRecord) (*DNSRecord, error) {
	env, err := fromDNSRecord(r)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, NewDataError("marshal", "DNS record", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, FormatURL(pathPolicies, c.Host, c.siteID), body)
	if err != nil {
		return nil, fmt.Errorf("creating DNS record: %w", err)
	}
	defer extdnshttp.DrainAndClose(resp.Body)

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewDataError("read", "create endpoint response body", err)
	}

	var createdEnv dnsPolicyEnvelope
	if err := json.Unmarshal(respBytes, &createdEnv); err != nil {
		return nil, NewDataError("unmarshal", "created DNS record", err)
	}

	out, ok := toDNSRecord(createdEnv)
	if !ok {
		return nil, NewDataError("convert", "created DNS record",
			fmt.Errorf("unsupported response type %q", createdEnv.Type))
	}

	return &out, nil
}

// DeleteRecord deletes a single DNS record by its ID. 404 responses are
// treated as success: the record is already gone, which is the desired
// outcome. Tolerating 404 lets callers use a snapshot-based index of records
// without worrying about staleness — concurrent deletes, manual cleanup in
// the UniFi UI, or a stale plan all converge harmlessly.
func (c *httpClient) DeleteRecord(ctx context.Context, id string) error {
	m := metrics.Get()
	start := time.Now()

	u := FormatURL(pathPolicy, c.Host, c.siteID, id)
	resp, err := c.doRequest(ctx, http.MethodDelete, u, nil)
	duration := time.Since(start)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			m.RecordUniFiAPICall("delete_record", duration, 0, nil)
			slog.Debug("record already deleted", "id", id)

			return nil
		}
		m.RecordUniFiAPICall("delete_record", duration, 0, err)

		return fmt.Errorf("deleting DNS record %s: %w", id, err)
	}
	extdnshttp.DrainAndClose(resp.Body)
	m.RecordUniFiAPICall("delete_record", duration, 0, nil)
	slog.Debug("client successfully removed record", "id", id)

	return nil
}
