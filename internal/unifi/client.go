package unifi

import (
	"bytes"
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
	"sigs.k8s.io/external-dns/endpoint"

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
)

// newUnifiClient creates a new DNS provider client and logs in to store cookies.
func newUnifiClient(config *Config) (*httpClient, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	// Create the HTTP client
	client := &httpClient{
		Config: config,
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTLSVerify},
			},
			Jar: jar,
		},
		ClientURLs: &ClientURLs{
			Login:   unifiLoginPath,
			Records: unifiRecordPath,
		},
	}

	if client.Config.ExternalController {
		client.ClientURLs.Login = unifiLoginPathExternal
		client.ClientURLs.Records = unifiRecordPathExternal
	}

	if client.Config.ApiKey != "" {
		return client, nil
	}

	log.Info("UNIFI_USER and UNIFI_PASSWORD are deprecated, please switch to using UNIFI_API_KEY instead")

	if err := client.login(); err != nil {
		return nil, err
	}

	return client, nil
}

// login performs a login request to the UniFi controller.
func (c *httpClient) login() error {
	m := metrics.Get()
	jsonBody, err := json.Marshal(Login{
		Username: c.Config.User,
		Password: c.Config.Password,
		Remember: true,
	})
	if err != nil {
		return err
	}

	// Perform the login request
	resp, err := c.doRequest(
		http.MethodPost,
		FormatUrl(c.ClientURLs.Login, c.Config.Host),
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		m.UniFiLoginTotal.WithLabelValues("failure").Inc()
		m.UniFiConnected.Set(0)
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		m.UniFiLoginTotal.WithLabelValues("failure").Inc()
		m.UniFiConnected.Set(0)
		respBody, _ := io.ReadAll(resp.Body)
		log.Error("login failed", zap.String("status", resp.Status), zap.String("response", string(respBody)))
		return errors.Newf("login failed: %s", resp.Status)
	}

	m.UniFiLoginTotal.WithLabelValues("success").Inc()
	m.UniFiConnected.Set(1)

	// Retrieve CSRF token from the response headers
	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
		m.UniFiCSRFRefreshesTotal.Inc()
	}
	return nil
}

func (c *httpClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	// TODO: Deprecation Notice - Use UNIFI_API_KEY instead
	if c.Config.ApiKey == "" {
		m := metrics.Get()
		if csrf := resp.Header.Get("X-CSRF-Token"); csrf != "" {
			if c.csrf != csrf {
				m.UniFiCSRFRefreshesTotal.Inc()
			}
			c.csrf = csrf
		}
		// If the status code is 401, re-login and retry the request
		if resp.StatusCode == http.StatusUnauthorized {
			m.UniFiReloginTotal.Inc()
			log.Debug("received 401 unauthorized, attempting to re-login")
			if err := c.login(); err != nil {
				log.Error("re-login failed", zap.Error(err))
				return nil, err
			}
			// Update the headers with new CSRF token
			c.setHeaders(req)

			// Retry the request
			log.Debug("retrying request after re-login")

			resp, err = c.Client.Do(req)
			if err != nil {
				log.Error("Retry request failed", zap.Error(err))
				return nil, err
			}
		}
	}

	// It is unknown at this time if the UniFi API returns anything other than 200 for these types of requests.
	if resp.StatusCode != http.StatusOK {
		body, bodyErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if bodyErr != nil {
			return nil, bodyErr
		}

		var apiError UnifiErrorResponse
		if err := json.Unmarshal(body, &apiError); err != nil {
			return nil, errors.Wrap(err, "failed to decode json")
		}

		return nil, errors.Newf("%s request to %s returned %d: %s", method, path, resp.StatusCode, apiError.Message)
	}

	return resp, nil
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *httpClient) GetEndpoints() ([]DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	resp, err := c.doRequest(
		http.MethodGet,
		FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site),
		nil,
	)

	duration := time.Since(start)

	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		m.RecordUniFiAPICall("get_endpoints", duration, 0, err)
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var records []DNSRecord
	if err = json.Unmarshal(bodyBytes, &records); err != nil {
		log.Error("Failed to decode response", zap.Error(err))
		m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), err)
		return nil, errors.Wrap(err, "failed to unmarshal DNS records")
	}

	m.RecordUniFiAPICall("get_endpoints", duration, len(bodyBytes), nil)

	// Loop through records to modify SRV type
	for i, record := range records {
		if record.RecordType != "SRV" {
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
func (c *httpClient) CreateEndpoint(endpoint *endpoint.Endpoint) ([]*DNSRecord, error) {
	m := metrics.Get()
	start := time.Now()

	if endpoint.RecordType == "CNAME" && len(endpoint.Targets) > 1 {
		m.IgnoredCNAMETargetsTotal.Inc()
		log.Warn("Ignoring additional CNAME targets. Only the first target will be used.", zap.String("key", endpoint.DNSName), zap.Strings("ignored_targets", endpoint.Targets[1:]))
		endpoint.Targets = endpoint.Targets[:1]
	}

	var createdRecords []*DNSRecord
	for _, target := range endpoint.Targets {
		record := DNSRecord{
			Enabled:    true,
			Key:        endpoint.DNSName,
			RecordType: endpoint.RecordType,
			TTL:        endpoint.RecordTTL,
			Value:      target,
		}

		if endpoint.RecordType == "SRV" {
			record.Priority = new(int)
			record.Weight = new(int)
			record.Port = new(int)

			if _, err := fmt.Sscanf(endpoint.Targets[0], "%d %d %d %s", record.Priority, record.Weight, record.Port, &record.Value); err != nil {
				m.SRVParsingErrorsTotal.Inc()
				duration := time.Since(start)
				m.RecordUniFiAPICall("create_endpoint", duration, 0, err)
				return nil, err
			}
		}

		jsonBody, err := json.Marshal(record)
		if err != nil {
			duration := time.Since(start)
			m.RecordUniFiAPICall("create_endpoint", duration, 0, err)
			return nil, err
		}

		resp, err := c.doRequest(
			http.MethodPost,
			FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site),
			bytes.NewReader(jsonBody),
		)
		if err != nil {
			duration := time.Since(start)
			m.RecordUniFiAPICall("create_endpoint", duration, 0, err)
			return nil, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			duration := time.Since(start)
			m.RecordUniFiAPICall("create_endpoint", duration, 0, err)
			return nil, errors.Wrap(err, "failed to read create endpoint response body")
		}

		var createdRecord DNSRecord
		if err = json.Unmarshal(bodyBytes, &createdRecord); err != nil {
			duration := time.Since(start)
			m.RecordUniFiAPICall("create_endpoint", duration, len(bodyBytes), err)
			return nil, errors.Wrap(err, "failed to unmarshal created DNS record")
		}

		createdRecords = append(createdRecords, &createdRecord)
		log.Debug("created new record", zap.Any("key", createdRecord.Key), zap.String("type", createdRecord.RecordType), zap.String("target", createdRecord.Value))
	}

	duration := time.Since(start)
	m.RecordUniFiAPICall("create_endpoint", duration, 0, nil)
	return createdRecords, nil
}

// DeleteEndpoint deletes a DNS record from the UniFi controller.
func (c *httpClient) DeleteEndpoint(endpoint *endpoint.Endpoint) error {
	m := metrics.Get()
	start := time.Now()

	records, err := c.GetEndpoints()
	if err != nil {
		duration := time.Since(start)
		m.RecordUniFiAPICall("delete_endpoint", duration, 0, err)
		return err
	}

	var deleteErrors []error
	for _, record := range records {
		if record.Key == endpoint.DNSName && record.RecordType == endpoint.RecordType {
			deleteURL := FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site, record.ID)

			_, err := c.doRequest(
				http.MethodDelete,
				deleteURL,
				nil,
			)
			if err != nil {
				deleteErrors = append(deleteErrors, err)
			} else {
				log.Debug("client successfully removed record", zap.String("key", record.Key), zap.String("type", record.RecordType), zap.String("target", record.Value))
			}
		}
	}

	duration := time.Since(start)
	if len(deleteErrors) > 0 {
		err := errors.Newf("failed to delete %d records: %v", len(deleteErrors), deleteErrors)
		m.RecordUniFiAPICall("delete_endpoint", duration, 0, err)
		return err
	}
	m.RecordUniFiAPICall("delete_endpoint", duration, 0, nil)
	return nil
}

// setHeaders sets the headers for the HTTP request.
func (c *httpClient) setHeaders(req *http.Request) {
	if c.Config.ApiKey != "" {
		req.Header.Set("X-API-KEY", c.Config.ApiKey)
	} else {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
}
