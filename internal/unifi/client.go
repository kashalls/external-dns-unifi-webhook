package unifi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"
)

// unifiAPIClient implements the UnifiAPI interface.
type unifiAPIClient struct {
	transport   HTTPTransport
	transformer RecordTransformer
	metrics     MetricsRecorder
	logger      Logger
	config      *Config
	clientURLs  *ClientURLs
}

const (
	unifiLoginPath          = "%s/api/auth/login"
	unifiLoginPathExternal  = "%s/api/login"
	unifiRecordPath         = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
	unifiRecordPathExternal = "%s/v2/api/site/%s/static-dns/%s"

	recordTypeA     = "A"
	recordTypeAAAA  = "AAAA"
	recordTypeCNAME = "CNAME"
	recordTypeMX    = "MX"
	recordTypeNS    = "NS"
	recordTypeSRV   = "SRV"
	recordTypeTXT   = "TXT"

	errorBodyBufferSize = 512
)

// NewUnifiAPIClient creates a new UniFi API client with injected dependencies.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func NewUnifiAPIClient(
	transport HTTPTransport,
	transformer RecordTransformer,
	metrics MetricsRecorder,
	logger Logger,
	config *Config,
	clientURLs *ClientURLs,
) UnifiAPI {
	return &unifiAPIClient{
		transport:   transport,
		transformer: transformer,
		metrics:     metrics,
		logger:      logger,
		config:      config,
		clientURLs:  clientURLs,
	}
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *unifiAPIClient) GetEndpoints(ctx context.Context) ([]DNSRecord, error) {
	start := time.Now()

	resp, err := c.transport.DoRequest(
		ctx,
		http.MethodGet,
		FormatURL(c.clientURLs.Records, c.config.Host, c.config.Site),
		nil,
	)

	duration := time.Since(start)

	if err != nil {
		c.metrics.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, errors.Wrap(err, "failed to fetch DNS records from UniFi")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.metrics.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, NewDataError("read", "get endpoints response body", err)
	}

	var records []DNSRecord
	err = json.Unmarshal(bodyBytes, &records)
	if err != nil {
		c.logger.Error("Failed to decode response", "error", err)
		c.metrics.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), err)

		return nil, NewDataError("unmarshal", "DNS records", err)
	}

	c.metrics.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), nil)

	// Loop through records to modify SRV type
	for i, record := range records {
		if record.RecordType != recordTypeSRV {
			continue
		}

		// Modify the Target for SRV records
		records[i].Value = c.transformer.FormatSRVValue(
			*record.Priority,
			*record.Weight,
			*record.Port,
			record.Value,
		)
		records[i].Priority = nil
		records[i].Weight = nil
		records[i].Port = nil
	}

	c.logger.Debug("fetched records", "count", len(records))

	return records, nil
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *unifiAPIClient) CreateEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) ([]*DNSRecord, error) {
	start := time.Now()

	// CNAME records can only have one target
	if endpoint.RecordType == recordTypeCNAME && len(endpoint.Targets) > 1 {
		c.metrics.IgnoredCNAMETargetsTotal().Inc()
		c.logger.Warn("Ignoring additional CNAME targets. Only the first target will be used.", "key", endpoint.DNSName, "ignored_targets", endpoint.Targets[1:])
		endpoint.Targets = endpoint.Targets[:1]
	}

	createdRecords := make([]*DNSRecord, 0, len(endpoint.Targets))

	for _, target := range endpoint.Targets {
		record := c.transformer.PrepareDNSRecord(endpoint, target)

		// SRV records need special parsing
		if endpoint.RecordType == recordTypeSRV {
			err := c.transformer.ParseSRVTarget(&record, endpoint.Targets[0])
			if err != nil {
				c.metrics.SRVParsingErrorsTotal().Inc()
				c.metrics.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

				return nil, errors.Wrap(err, "failed to parse SRV target")
			}
		}

		createdRecord, err := c.createSingleDNSRecord(ctx, &record)
		if err != nil {
			c.metrics.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

			return nil, err
		}

		createdRecords = append(createdRecords, createdRecord)
		c.logger.Debug("created new record", "key", createdRecord.Key, "type", createdRecord.RecordType, "target", createdRecord.Value)
	}

	c.metrics.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, nil)

	return createdRecords, nil
}

// createSingleDNSRecord sends a create request for a single DNS record and returns the created record.
func (c *unifiAPIClient) createSingleDNSRecord(ctx context.Context, record *DNSRecord) (*DNSRecord, error) {
	jsonBody, err := json.Marshal(record)
	if err != nil {
		return nil, NewDataError("marshal", "DNS record", err)
	}

	resp, err := c.transport.DoRequest(
		ctx,
		http.MethodPost,
		FormatURL(c.clientURLs.Records, c.config.Host, c.config.Site),
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create DNS record")
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, NewDataError("read", "create endpoint response body", err)
	}

	var createdRecord DNSRecord
	err = json.Unmarshal(bodyBytes, &createdRecord)
	if err != nil {
		return nil, NewDataError("unmarshal", "created DNS record", err)
	}

	return &createdRecord, nil
}

// DeleteEndpoint deletes a DNS record from the UniFi controller.
func (c *unifiAPIClient) DeleteEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) error {
	start := time.Now()

	records, err := c.GetEndpoints(ctx)
	if err != nil {
		duration := time.Since(start)
		c.metrics.RecordUniFiAPICall("delete_endpoint", duration, 0, err)

		return errors.Wrap(err, "failed to fetch records before deletion")
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key == endpoint.DNSName && record.RecordType == endpoint.RecordType {
			deleteURL := FormatURL(c.clientURLs.Records, c.config.Host, c.config.Site, record.ID)

			resp, err := c.transport.DoRequest(
				ctx,
				http.MethodDelete,
				deleteURL,
				nil,
			)
			if err != nil {
				deleteErrors = append(deleteErrors, err)
			} else {
				_ = resp.Body.Close()
				c.logger.Debug("client successfully removed record", "key", record.Key, "type", record.RecordType, "target", record.Value)
			}
		}
	}

	duration := time.Since(start)
	if len(deleteErrors) > 0 {
		err := errors.Newf("failed to delete %d records", len(deleteErrors))
		for _, deleteErr := range deleteErrors {
			err = errors.Wrap(deleteErr, err.Error())
		}
		c.metrics.RecordUniFiAPICall("delete_endpoint", duration, 0, err)

		return err
	}
	c.metrics.RecordUniFiAPICall("delete_endpoint", duration, 0, nil)

	return nil
}

// newUnifiClient creates a new DNS provider client - DEPRECATED, use NewUnifiAPIClient instead.
// This function is kept for backward compatibility and will be removed in future versions.
//
//nolint:ireturn // Factory function must return interface for dependency injection
func newUnifiClient(config *Config) (UnifiAPI, error) {
	metricsAdapter := NewMetricsAdapter(nil)
	loggerAdapter := NewLoggerAdapter()

	transport, err := NewHTTPTransport(config, metricsAdapter, loggerAdapter)
	if err != nil {
		return nil, err
	}

	// Get ClientURLs from transport
	var clientURLs *ClientURLs
	if ht, ok := transport.(interface{ GetClientURLs() *ClientURLs }); ok {
		clientURLs = ht.GetClientURLs()
	} else {
		// Fallback if transport doesn't expose GetClientURLs
		clientURLs = &ClientURLs{
			Login:   unifiLoginPath,
			Records: unifiRecordPath,
		}
		if config.ExternalController {
			clientURLs.Login = unifiLoginPathExternal
			clientURLs.Records = unifiRecordPathExternal
		}
	}

	transformer := NewRecordTransformer()

	return NewUnifiAPIClient(
		transport,
		transformer,
		metricsAdapter,
		loggerAdapter,
		config,
		clientURLs,
	), nil
}
