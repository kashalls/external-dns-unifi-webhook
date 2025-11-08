package unifi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"golang.org/x/net/publicsuffix"
	externaldnsendpoint "sigs.k8s.io/external-dns/endpoint"

	"go.uber.org/zap"
)

type ClientURLs struct {
	Login   string
	Records string
}

// httpClient is the DNS provider client.
type httpClient struct {
	*Config
	*http.Client

	csrf       string
	ClientURLs *ClientURLs
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

// newUnifiClient creates a new DNS provider client and logs in to store cookies.
func newUnifiClient(config *Config) (*httpClient, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cookie jar")
	}

	// Create the HTTP client
	client := &httpClient{
		Config: config,
		Client: &http.Client{
			Transport: &http.Transport{
				//nolint:gosec // InsecureSkipVerify is configurable via UNIFI_SKIP_TLS_VERIFY for self-signed certs
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTLSVerify},
			},
			Jar: jar,
		},
		ClientURLs: &ClientURLs{
			Login:   unifiLoginPath,
			Records: unifiRecordPath,
		},
	}

	if client.ExternalController {
		client.ClientURLs.Login = unifiLoginPathExternal
		client.ClientURLs.Records = unifiRecordPathExternal
	}

	if client.APIKey != "" {
		return client, nil
	}

	log.Info("UNIFI_USER and UNIFI_PASSWORD are deprecated, please switch to using UNIFI_API_KEY instead")

	err = client.login(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "initial login failed")
	}

	return client, nil
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *httpClient) GetEndpoints(ctx context.Context) ([]DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	resp, err := c.doRequest(
		ctx,
		http.MethodGet,
		FormatURL(c.ClientURLs.Records, c.Host, c.Site),
		nil,
	)

	duration := time.Since(start)

	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, errors.Wrap(err, "failed to fetch DNS records from UniFi")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)

		return nil, NewDataError("read", "get endpoints response body", err)
	}

	var records []DNSRecord
	err = json.Unmarshal(bodyBytes, &records)
	if err != nil {
		log.Error("Failed to decode response", zap.Error(err))
		m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), err)

		return nil, NewDataError("unmarshal", "DNS records", err)
	}

	m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), nil)

	// Loop through records to modify SRV type
	for i, record := range records {
		if record.RecordType != recordTypeSRV {
			continue
		}

		// Modify the Target for SRV records
		records[i].Value = fmt.Sprintf("%d %d %d %s",
			*record.Priority,
			*record.Weight,
			*record.Port,
			record.Value,
		)
		records[i].Priority = nil
		records[i].Weight = nil
		records[i].Port = nil
	}

	log.Debug("fetched records", zap.Int("count", len(records)))

	return records, nil
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *httpClient) CreateEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) ([]*DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	// CNAME records can only have one target
	if endpoint.RecordType == recordTypeCNAME && len(endpoint.Targets) > 1 {
		m.IgnoredCNAMETargetsTotal.WithLabelValues(metrics.ProviderName).Inc()
		log.Warn("Ignoring additional CNAME targets. Only the first target will be used.", zap.String("key", endpoint.DNSName), zap.Strings("ignored_targets", endpoint.Targets[1:]))
		endpoint.Targets = endpoint.Targets[:1]
	}

	createdRecords := make([]*DNSRecord, 0, len(endpoint.Targets))

	for _, target := range endpoint.Targets {
		record := prepareDNSRecord(endpoint, target)

		// SRV records need special parsing
		if endpoint.RecordType == recordTypeSRV {
			err := parseSRVTarget(&record, endpoint.Targets[0])
			if err != nil {
				m.SRVParsingErrorsTotal.WithLabelValues(metrics.ProviderName).Inc()
				m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

				return nil, err
			}
		}

		createdRecord, err := c.createSingleDNSRecord(ctx, record)
		if err != nil {
			m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, err)

			return nil, err
		}

		createdRecords = append(createdRecords, createdRecord)
		log.Debug("created new record", zap.Any("key", createdRecord.Key), zap.String("type", createdRecord.RecordType), zap.String("target", createdRecord.Value))
	}

	m.RecordUniFiAPICall("create_endpoint", time.Since(start), 0, nil)

	return createdRecords, nil
}

// prepareDNSRecord creates a DNSRecord from endpoint and target value.
func prepareDNSRecord(endpoint *externaldnsendpoint.Endpoint, target string) DNSRecord {
	return DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      target,
	}
}

// parseSRVTarget parses SRV record format and populates Priority, Weight, Port fields.
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

// createSingleDNSRecord sends a create request for a single DNS record and returns the created record.
func (c *httpClient) createSingleDNSRecord(ctx context.Context, record DNSRecord) (*DNSRecord, error) {
	jsonBody, err := json.Marshal(record)
	if err != nil {
		return nil, NewDataError("marshal", "DNS record", err)
	}

	resp, err := c.doRequest(
		ctx,
		http.MethodPost,
		FormatURL(c.ClientURLs.Records, c.Host, c.Site),
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
func (c *httpClient) DeleteEndpoint(ctx context.Context, endpoint *externaldnsendpoint.Endpoint) error {
	m := metrics.Get()
	start := time.Now()

	records, err := c.GetEndpoints(ctx)
	if err != nil {
		duration := time.Since(start)
		m.RecordUniFiAPICall("delete_endpoint", duration, 0, err)

		return errors.Wrap(err, "failed to fetch records before deletion")
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key == endpoint.DNSName && record.RecordType == endpoint.RecordType {
			deleteURL := FormatURL(c.ClientURLs.Records, c.Host, c.Site, record.ID)

			resp, err := c.doRequest(
				ctx,
				http.MethodDelete,
				deleteURL,
				nil,
			)
			if err != nil {
				deleteErrors = append(deleteErrors, err)
			} else {
				_ = resp.Body.Close()
				log.Debug("client successfully removed record", zap.String("key", record.Key), zap.String("type", record.RecordType), zap.String("target", record.Value))
			}
		}
	}

	duration := time.Since(start)
	if len(deleteErrors) > 0 {
		err := errors.Newf("failed to delete %d records", len(deleteErrors))
		for _, deleteErr := range deleteErrors {
			err = errors.Wrap(deleteErr, err.Error())
		}
		m.RecordUniFiAPICall("delete_endpoint", duration, 0, err)

		return err
	}
	m.RecordUniFiAPICall("delete_endpoint", duration, 0, nil)

	return nil
}

func (c *httpClient) login(ctx context.Context) error {
	m := metrics.Get()
	jsonBody, err := json.Marshal(Login{
		Username: c.User,
		Password: c.Password,
		Remember: true,
	})
	if err != nil {
		return NewDataError("marshal", "login credentials", err)
	}

	// Perform the login request
	resp, err := c.doRequest(
		ctx,
		http.MethodPost,
		FormatURL(c.ClientURLs.Login, c.Host),
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		m.UniFiLoginTotal.WithLabelValues(metrics.ProviderName, "failure").Inc()
		m.UniFiConnected.WithLabelValues(metrics.ProviderName).Set(0)

		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		m.UniFiLoginTotal.WithLabelValues(metrics.ProviderName, "failure").Inc()
		m.UniFiConnected.WithLabelValues(metrics.ProviderName).Set(0)
		respBody, readErr := io.ReadAll(resp.Body)
		responseMsg := ""
		if readErr == nil {
			responseMsg = string(respBody)
		}
		log.Error("login failed", zap.String("status", resp.Status), zap.String("response", responseMsg))

		return NewAuthError("login", resp.StatusCode, resp.Status, nil)
	}

	m.UniFiLoginTotal.WithLabelValues(metrics.ProviderName, "success").Inc()
	m.UniFiConnected.WithLabelValues(metrics.ProviderName).Set(1)

	// Retrieve CSRF token from the response headers
	if csrf := resp.Header.Get("X-Csrf-Token"); csrf != "" {
		c.csrf = resp.Header.Get("X-Csrf-Token")
		m.UniFiCSRFRefreshesTotal.WithLabelValues(metrics.ProviderName).Inc()
	}

	return nil
}

func (c *httpClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	c.setHeaders(req)

	resp, err := c.Do(req)
	if err != nil {
		return nil, NewNetworkError(method, path, err)
	}

	// TODO: Deprecation Notice - Use UNIFI_API_KEY instead
	//nolint:godox // This TODO is intentional and will remain until the deprecated auth method is removed
	if c.APIKey == "" {
		c.handleCSRFToken(resp)

		// If the status code is 401, re-login and retry the request
		if resp.StatusCode == http.StatusUnauthorized {
			resp, err = c.handleUnauthorized(ctx, req, method, path)
			if err != nil {
				return nil, err
			}
		}
	}

	// It is unknown at this time if the UniFi API returns anything other than 200 for these types of requests.
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp, method, path)
	}

	return resp, nil
}

// handleCSRFToken updates the CSRF token from response headers.
func (c *httpClient) handleCSRFToken(resp *http.Response) {
	csrf := resp.Header.Get("X-Csrf-Token")
	if csrf == "" {
		return
	}

	if c.csrf != csrf {
		metrics.Get().UniFiCSRFRefreshesTotal.WithLabelValues(metrics.ProviderName).Inc()
	}
	c.csrf = csrf
}

// handleUnauthorized handles 401 responses by re-logging in and retrying the request.
func (c *httpClient) handleUnauthorized(ctx context.Context, req *http.Request, method, path string) (*http.Response, error) {
	met := metrics.Get()
	met.UniFiReloginTotal.WithLabelValues(metrics.ProviderName).Inc()

	log.Debug("received 401 unauthorized, attempting to re-login")

	err := c.login(ctx)
	if err != nil {
		log.Error("re-login failed", zap.Error(err))

		return nil, errors.Wrap(err, "re-login after 401 failed")
	}

	// Update the headers with new CSRF token
	c.setHeaders(req)

	// Retry the request
	log.Debug("retrying request after re-login")

	resp, err := c.Do(req)
	if err != nil {
		log.Error("Retry request failed", zap.Error(err))

		return nil, NewNetworkError(method+" (retry)", path, err)
	}

	return resp, nil
}

// handleErrorResponse processes non-200 status codes and returns appropriate errors.
func (c *httpClient) handleErrorResponse(resp *http.Response, method, path string) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, errorBodyBufferSize))
	if err != nil {
		return NewDataError("read", "error response body", err)
	}

	var apiError UnifiErrorResponse
	err = json.Unmarshal(bodyBytes, &apiError)
	if err != nil {
		return NewDataError("unmarshal", "API error response", err)
	}

	return NewAPIError(method, path, resp.StatusCode, apiError.Message)
}

// setHeaders sets the headers for the HTTP request.
func (c *httpClient) setHeaders(req *http.Request) {
	if c.APIKey != "" {
		req.Header.Set("X-Api-Key", c.APIKey)
	} else {
		req.Header.Set("X-Csrf-Token", c.csrf)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
}
