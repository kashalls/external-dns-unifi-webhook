package unifi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	unifiLoginPath  = "%s/api/auth/login"
	unifiRecordPath = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
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
		FormatUrl(unifiLoginPath, c.Config.Host),
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Error("login failed", zap.String("status", resp.Status), zap.String("response", string(respBody)))
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	// Retrieve CSRF token from the response headers
	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
	}

	return nil
}

// doRequest makes an HTTP request to the UniFi controller.
func (c *httpClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	log.With(
		zap.String("req_method", method),
		zap.String("req_path", path),
		zap.Any("req_body", body),
	).Debug("Creating Request")

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("X-CSRF-Token"); csrf != "" {
		c.csrf = csrf
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response body", zap.Error(err))
		return resp, nil
	}
	bodyString := string(bodyBytes)

	log.With(zap.String("req_method", method), zap.String("req_path", path), zap.Int("req_code", resp.StatusCode), zap.String("req_body", bodyString)).Debug("Returned Request")

	// If the status code is 401, re-login and retry the request
	if resp.StatusCode == http.StatusUnauthorized {
		log.Debug("Received 401 Unauthorized, re-login required")
		if err := c.login(); err != nil {
			return nil, err
		}
		// Update the headers with new CSRF token
		c.setHeaders(req)
		// Retry the request
		resp, err = c.Client.Do(req)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s request to %s was not successful: %d", method, path, resp.StatusCode)
	}

	return resp, nil
}

// GetEndpoints retrieves the list of DNS records from the UniFi controller.
func (c *httpClient) GetEndpoints() ([]DNSRecord, error) {
	url := FormatUrl(unifiRecordPath, c.Config.Host, c.Config.Site)
	resp, err := c.doRequest(http.MethodGet, url, nil)
	if err != nil {
		log.With(zap.Error(err), zap.String("req_url", url)).Debug("Endpoint Request Failed")
		return nil, err
	}
	defer resp.Body.Close()

	var records []DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&records); err != nil {
		log.With(zap.Error(err), zap.String("req_url", url), zap.Any("req_body", resp.Body)).Debug("JSON Encoding Error")
		return nil, err
	}

	log.Debug(fmt.Sprintf("retrieved records: %+v", records))

	return records, nil
}

// CreateEndpoint creates a new DNS record in the UniFi controller.
func (c *httpClient) CreateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	jsonBody, err := json.Marshal(DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      endpoint.Targets[0],
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(
		http.MethodPost,
		FormatUrl(unifiRecordPath, c.Config.Host, c.Config.Site),
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var record DNSRecord
	if err = json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, err
	}

	log.Debug(fmt.Sprintf("created record: %+v", record))

	return &record, nil
}

// DeleteEndpoint deletes a DNS record from the UniFi controller.
func (c *httpClient) DeleteEndpoint(endpoint *endpoint.Endpoint) error {
	lookup, err := c.lookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		return err
	}

	if _, err = c.doRequest(
		http.MethodDelete,
		FormatUrl(unifiRecordPath, c.Config.Host, c.Config.Site, lookup.ID),
		nil,
	); err != nil {
		return err
	}

	return nil
}

// lookupIdentifier finds the ID of a DNS record in the UniFi controller.
func (c *httpClient) lookupIdentifier(key, recordType string) (*DNSRecord, error) {
	records, err := c.GetEndpoints()
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Key == key && r.RecordType == recordType {
			return &r, nil
		}
	}

	return nil, err
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
		log.Debug(fmt.Sprintf("Requesting %s cookies: %d", req.URL, len(c.Client.Jar.Cookies(parsedURL))))
	} else {
		log.Debug(fmt.Sprintf("Requesting %s", req.URL))
	}
}
