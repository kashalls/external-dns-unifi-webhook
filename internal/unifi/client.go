package unifi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/log"
	"golang.org/x/net/publicsuffix"
	"sigs.k8s.io/external-dns/endpoint"

	"go.uber.org/zap"
)

// httpClient is the DNS provider client.
type httpClient struct {
	*Config
	*http.Client
	csrf string
}

const (
	unifiLoginPathUDM         = "%s/api/auth/login"
	unifiLoginPathNetworkApp  = "%s/api/login"
	unifiRecordPathUDM        = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
	unifiRecordPathNetworkApp = "%s/v2/api/site/%s/static-dns/%s"
)

// newUnifiClient creates a new DNS provider client and logs in to store cookies.
func newUnifiClient(config *Config) (*httpClient, error) {
	log.Debug("Creating new UniFi client", zap.String("host", config.Host), zap.String("controllerType", config.ControllerType))

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		log.Error("Failed to create cookie jar", zap.Error(err))
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
	}

	log.Debug("Attempting to login")
	if err := client.login(); err != nil {
		log.Error("Failed to login", zap.Error(err))
		return nil, err
	}

	log.Debug("UniFi client created and logged in successfully")
	return client, nil
}

// login performs a login request to the UniFi controller.
func (c *httpClient) login() error {
	loginPath := unifiLoginPathUDM
	if c.Config.ControllerType == "NETWORK_SERVER" {
		loginPath = unifiLoginPathNetworkApp
	}
	log.Debug("Logging in", zap.String("loginPath", loginPath))

	jsonBody, err := json.Marshal(Login{
		Username: c.Config.User,
		Password: c.Config.Password,
		Remember: true,
	})
	if err != nil {
		log.Error("Failed to marshal login JSON", zap.Error(err))
		return err
	}

	// Print request details
	log.Debug("Sending login request",
		zap.String("URL", FormatUrl(loginPath, c.Config.Host)),
		zap.String("body", string(jsonBody)))

	// Perform the login request
	resp, err := c.doRequest(
		http.MethodPost,
		FormatUrl(loginPath, c.Config.Host),
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		log.Error("Login request failed", zap.Error(err))
		return err
	}

	defer resp.Body.Close()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Error("Login failed",
			zap.String("status", resp.Status),
			zap.String("response", string(respBody)))
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	// Retrieve CSRF token from the response headers
	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
		log.Debug("Retrieved CSRF token", zap.String("token", c.csrf))
	} else {
		log.Debug("No CSRF token found in response headers")
	}

	log.Debug("Login successful", zap.String("status", resp.Status))
	return nil
}

func (c *httpClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	log.Debug("Making request", zap.String("method", method), zap.String("path", path))

	// Convert body to bytes for logging and reuse
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = io.ReadAll(body)
		body = bytes.NewReader(bodyBytes)
		log.Debug("Request body", zap.String("body", string(bodyBytes)))
	}

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		log.Error("Failed to create request", zap.Error(err))
		return nil, err
	}
	// Set the required headers
	if body != nil {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	}
	// Dynamically set the Host header
	parsedURL, err := url.Parse(path)
	if err != nil {
		log.Error("Failed to parse URL", zap.Error(err))
		return nil, err
	}
	req.Host = parsedURL.Host

	log.Debug("Request host", zap.String("host", req.Host))

	c.setHeaders(req)

	// Log all request headers
	for name, values := range req.Header {
		for _, value := range values {
			log.Debug("Request header", zap.String("name", name), zap.String("value", value))
		}
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		log.Error("Request failed", zap.Error(err))
		return nil, err
	}

	// Log all response headers
	for name, values := range resp.Header {
		for _, value := range values {
			log.Debug("Response header", zap.String("name", name), zap.String("value", value))
		}
	}

	// Log response body
	respBody, _ := io.ReadAll(resp.Body)
	log.Debug("Response body", zap.String("body", string(respBody)))
	resp.Body = io.NopCloser(bytes.NewBuffer(respBody)) // Restore the response body for further use

	if csrf := resp.Header.Get("X-CSRF-Token"); csrf != "" {
		c.csrf = csrf
		log.Debug("Updated CSRF token", zap.String("token", c.csrf))
	}

	log.Debug("Response received",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("statusCode", resp.StatusCode))

	// If the status code is 401, re-login and retry the request
	if resp.StatusCode == http.StatusUnauthorized {
		log.Debug("Received 401 Unauthorized, attempting to re-login")
		if err := c.login(); err != nil {
			log.Error("Re-login failed", zap.Error(err))
			return nil, err
		}
		// Update the headers with new CSRF token
		c.setHeaders(req)
		// Retry the request
		log.Debug("Retrying request after re-login")
		resp, err = c.Client.Do(req)
		if err != nil {
			log.Error("Retry request failed", zap.Error(err))
			return nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Request was not successful",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("statusCode", resp.StatusCode))
		return nil, fmt.Errorf("%s request to %s was not successful: %d", method, path, resp.StatusCode)
	}

	return resp, nil
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *httpClient) GetEndpoints() ([]DNSRecord, error) {
	log.Debug("Getting endpoints")
	recordPath := unifiRecordPathUDM
	if c.Config.ControllerType == "NETWORK_SERVER" {
		recordPath = unifiRecordPathNetworkApp
	}

	resp, err := c.doRequest(
		http.MethodGet,
		FormatUrl(recordPath, c.Config.Host, c.Config.Site),
		nil,
	)
	if err != nil {
		log.Error("Failed to get endpoints", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	var records []DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&records); err != nil {
		log.Error("Failed to decode response", zap.Error(err))
		return nil, err
	}

	log.Debug("Retrieved records", zap.Int("count", len(records)))
	for _, record := range records {
		log.Debug("Record", zap.Any("record", record))
	}

	return records, nil
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *httpClient) CreateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	log.Debug("Creating endpoint", zap.String("dnsName", endpoint.DNSName))
	recordPath := unifiRecordPathUDM
	if c.Config.ControllerType == "NETWORK_SERVER" {
		recordPath = unifiRecordPathNetworkApp
	}

	record := DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      endpoint.Targets[0],
	}

	jsonBody, err := json.Marshal(record)
	if err != nil {
		log.Error("Failed to marshal record", zap.Error(err))
		return nil, err
	}
	log.Debug("Creating record", zap.String("record", string(jsonBody)))

	resp, err := c.doRequest(
		http.MethodPost,
		FormatUrl(recordPath, c.Config.Host, c.Config.Site),
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		log.Error("Failed to create endpoint", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	var createdRecord DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&createdRecord); err != nil {
		log.Error("Failed to decode response", zap.Error(err))
		return nil, err
	}

	log.Debug("Created record", zap.Any("record", createdRecord))

	return &createdRecord, nil
}

// DeleteEndpoint deletes a DNS record from the UniFi controller.
func (c *httpClient) DeleteEndpoint(endpoint *endpoint.Endpoint) error {
	log.Debug("Deleting endpoint", zap.String("dnsName", endpoint.DNSName))
	lookup, err := c.lookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		log.Error("Failed to lookup identifier", zap.Error(err))
		return err
	}

	recordPath := unifiRecordPathUDM
	if c.Config.ControllerType == "NETWORK_SERVER" {
		recordPath = unifiRecordPathNetworkApp
	}

	deleteURL := FormatUrl(recordPath, c.Config.Host, c.Config.Site, lookup.ID)
	log.Debug("Deleting record", zap.String("url", deleteURL))

	_, err = c.doRequest(
		http.MethodDelete,
		deleteURL,
		nil,
	)
	if err != nil {
		log.Error("Failed to delete endpoint", zap.Error(err))
		return err
	}

	log.Debug("Successfully deleted endpoint", zap.String("dnsName", endpoint.DNSName))
	return nil
}

// lookupIdentifier finds the ID of a DNS record in the UniFi controller.
func (c *httpClient) lookupIdentifier(key, recordType string) (*DNSRecord, error) {
	log.Debug("Looking up identifier", zap.String("key", key), zap.String("recordType", recordType))
	records, err := c.GetEndpoints()
	if err != nil {
		log.Error("Failed to get endpoints", zap.Error(err))
		return nil, err
	}

	for _, r := range records {
		if r.Key == key && r.RecordType == recordType {
			log.Debug("Found matching record", zap.Any("record", r))
			return &r, nil
		}
	}

	log.Debug("Record not found", zap.String("key", key), zap.String("recordType", recordType))
	return nil, fmt.Errorf("record not found")
}

// setHeaders sets the headers for the HTTP request.
func (c *httpClient) setHeaders(req *http.Request) {
	log.Debug("Setting headers for request")
	// Add the saved CSRF header.
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")

	// Log the request URL and cookies
	if c.Client.Jar != nil {
		parsedURL, _ := url.Parse(req.URL.String())
		cookies := c.Client.Jar.Cookies(parsedURL)
		log.Debug("Request cookies",
			zap.String("url", req.URL.String()),
			zap.Int("cookieCount", len(cookies)))
		for _, cookie := range cookies {
			log.Debug("Cookie", zap.String("name", cookie.Name), zap.String("value", cookie.Value))
		}
	} else {
		log.Debug("No cookie jar available", zap.String("url", req.URL.String()))
	}
}
