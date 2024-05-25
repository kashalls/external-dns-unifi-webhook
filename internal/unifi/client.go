package unifi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/publicsuffix"
	"sigs.k8s.io/external-dns/endpoint"
)

// httpClient is the DNS provider client.
type httpClient struct {
	config *Config
	hc     *http.Client
	csrf   string
}

const (
	unifiLoginPath   = "%s/api/auth/login"
	unifiRecordsPath = "%s/proxy/network/v2/api/site/default/static-dns"
	unifiRecordPath  = "%s/proxy/network/v2/api/site/default/static-dns/%s"
)

// newUnifiClient creates a new DNS provider client and logs in to store cookies.
func newUnifiClient(config *Config) (*httpClient, error) {
	// Create a cookie jar to store the CSRF token
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	// Create the HTTP client
	client := &httpClient{
		config: config,
		hc: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTLSVerify},
			},
			Jar: jar,
		},
		csrf: "",
	}

	if err := client.login(); err != nil {
		return nil, err
	}

	return client, nil
}

// login performs a login request to the UniFi controller.
func (c *httpClient) login() error {
	// Prepare the login request body
	body, err := json.Marshal(map[string]string{
		"username": c.config.User,
		"password": c.config.Password,
	})
	if err != nil {
		return err
	}

	// Perform the login request
	resp, err := c.hc.Post(fmt.Sprintf(unifiLoginPath, c.config.Host), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check if the login was successful
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Errorf("login failed: %s, response: %s", resp.Status, string(respBody))
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	// Retrieve CSRF token from the response headers
	c.csrf = resp.Header.Get("X-CSRF-Token")
	if c.csrf == "" {
		return fmt.Errorf("login failed: CSRF token not found")
	}

	return nil
}

// doRequest makes an HTTP request to the UniFi controller.
func (c *httpClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	log.Debugf("making %s request to /%s", method, path)

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("X-CSRF-Token"); csrf != "" {
		c.csrf = csrf
	}

	log.Debugf("response code from %s request to %s: %d", method, path, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s request to %s was not successful: %d", method, path, resp.StatusCode)
	}

	return resp, nil
}

// GetEndpoints retrieves the list of DNS records.
func (c *httpClient) GetEndpoints() ([]DNSRecord, error) {
	resp, err := c.doRequest(http.MethodGet, fmt.Sprintf(unifiRecordsPath, c.config.Host), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var records []DNSRecord
	err = json.NewDecoder(resp.Body).Decode(&records)
	if err != nil {
		return nil, err
	}
	log.Debugf("retrieved records: %+v", records)

	return records, nil
}

// CreateEndpoint creates a new DNS record.
func (c *httpClient) CreateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	jsonBody, err := json.Marshal(DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      endpoint.Targets[0],
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DNS record: %w", err)
	}

	bodyReader := bytes.NewReader(jsonBody)
	resp, err := c.doRequest(http.MethodPost, fmt.Sprintf(unifiRecordsPath, c.config.Host), bodyReader)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var record DNSRecord
	err = json.NewDecoder(resp.Body).Decode(&record)
	if err != nil {
		return nil, err
	}
	log.Debugf("created record: %+v", record)

	return &record, nil
}

// DeleteEndpoint deletes a DNS record.
func (c *httpClient) DeleteEndpoint(endpoint *endpoint.Endpoint) error {
	lookup, err := c.LookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		return err
	}

	_, err = c.doRequest(
		http.MethodPost,
		fmt.Sprintf(unifiRecordPath, c.config.Host, lookup.ID),
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}

// LookupIdentifier finds the ID of a DNS record.
func (c *httpClient) LookupIdentifier(key, recordType string) (*DNSRecord, error) {
	records, err := c.GetEndpoints()
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Key == key && r.RecordType == recordType {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("record not found")
}
