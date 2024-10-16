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
	unifiLoginPathGateway     = "%s/api/auth/login"
	unifiLoginPathStandalone  = "%s/api/login"
	unifiRecordPathGateway    = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
	unifiRecordPathStandalone = "%s/v2/api/site/%s/static-dns/%s"
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
			Login:   unifiLoginPathGateway,
			Records: unifiRecordPathGateway,
		},
	}

	if config.ExternalController {
		client.ClientURLs.Login = unifiLoginPathStandalone
		client.ClientURLs.Records = unifiRecordPathStandalone
	}

	if err := client.login(); err != nil {
		return nil, err
	}

	return client, nil
}

// login performs a login request to the UniFi controller.
func (c *httpClient) login() error {
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
	}
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

	resp, err := c.Client.Do(req)
	if err != nil {
		log.Error("Request failed", zap.Error(err))
		return nil, err
	}

	// Log response body
	respBody, _ := io.ReadAll(resp.Body)
	log.Debug("response body", zap.String("body", string(respBody)))
	resp.Body = io.NopCloser(bytes.NewBuffer(respBody)) // Restore the response body for further use

	if csrf := resp.Header.Get("X-CSRF-Token"); csrf != "" {
		c.csrf = csrf
		log.Debug("Updated CSRF token", zap.String("token", c.csrf))
	}

	log.Debug("recieved response", zap.String("method", method), zap.String("path", path), zap.Int("statusCode", resp.StatusCode))

	// If the status code is 401, re-login and retry the request
	if resp.StatusCode == http.StatusUnauthorized {
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

	resp, err := c.doRequest(
		http.MethodGet,
		FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site),
		nil,
	)
	if err != nil {
		log.Error("failed to get endpoints", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	var records []DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&records); err != nil {
		log.Error("Failed to decode response", zap.Error(err))
		return nil, err
	}

	log.Debug("retrieved records", zap.Int("count", len(records)))

	return records, nil
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *httpClient) CreateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	log.Debug("Creating endpoint", zap.String("key", endpoint.DNSName))

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
		FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site),
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
	lookup, err := c.lookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		return err
	}

	deleteURL := FormatUrl(c.ClientURLs.Records, c.Config.Host, c.Config.Site, lookup.ID)

	_, err = c.doRequest(
		http.MethodDelete,
		deleteURL,
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}

// lookupIdentifier finds the ID of a DNS record in the UniFi controller.
func (c *httpClient) lookupIdentifier(key, recordType string) (*DNSRecord, error) {
	log.Debug("Looking up identifier", zap.String("key", key), zap.String("recordType", recordType))
	records, err := c.GetEndpoints()
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Key == key && r.RecordType == recordType {
			log.Debug("Found matching record", zap.Any("record", r))
			return &r, nil
		}
	}

	return nil, fmt.Errorf("record not found: %s", key)
}

// setHeaders sets the headers for the HTTP request.
func (c *httpClient) setHeaders(req *http.Request) {
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
	} else {
		log.Debug("No cookie jar available", zap.String("url", req.URL.String()))
	}
}
