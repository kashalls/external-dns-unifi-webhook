package unifi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	"sigs.k8s.io/external-dns/endpoint"
)

// Client is the DNS provider client.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	csrf       string
}

// DNSRecord represents a DNS record in the API.
type DNSRecord struct {
	ID         string       `json:"_id,omitempty"`
	Enabled    bool         `json:"enabled,omitempty"`
	Key        string       `json:"key"`
	Port       int          `json:"port,omitempty"`
	Priority   int          `json:"priority,omitempty"`
	RecordType string       `json:"record_type"`
	TTL        endpoint.TTL `json:"ttl,omitempty"`
	Value      string       `json:"value"`
	Weight     int          `json:"weight,omitempty"`
}

var (
	UnifiLogin           = "%s/api/auth/login"
	UnifiDNSRecords      = "%s/proxy/network/v2/api/site/default/static-dns"
	UnifiDNSSelectRecord = "%s/proxy/network/v2/api/site/default/static-dns/%s"
)

// newUnifiClient creates a new DNS provider client and logs in to store cookies.
func newUnifiClient(config *Configuration) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.SkipTLSVerify},
	}

	client := &Client{
		BaseURL: config.Host,
		HTTPClient: &http.Client{
			Transport: transport,
			Jar:       jar,
		},
	}

	if err := client.login(config.User, config.Password); err != nil {
		return nil, err
	}

	return client, nil
}

// login authenticates the client and stores the cookies.
func (c *Client) login(username, password string) error {
	loginURL := fmt.Sprintf(UnifiLogin, c.BaseURL)

	credentials := map[string]string{
		"username": username,
		"password": password,
	}

	body, err := json.Marshal(credentials)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(loginURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", resp.Status)
	}

	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
}

func (c *Client) GetData(url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf(url, c.BaseURL), nil)
	c.setHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
	}

	defer resp.Body.Close()

	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return byteArray, nil
}

func (c *Client) ShipData(url string, body []byte) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf(url, c.BaseURL), bytes.NewBuffer(body))
	c.setHeaders(req)

	resp, err := c.HTTPClient.Do(req)

	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
	}

	defer resp.Body.Close()

	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return byteArray, nil
}

func (c *Client) PutData(url string, body []byte) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	c.setHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
	}

	defer resp.Body.Close()

	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return byteArray, nil
}

func (c *Client) DeleteData(url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodDelete, url, nil)

	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)

	if err != nil {
		return nil, err
	}

	if csrf := resp.Header.Get("x-csrf-token"); csrf != "" {
		c.csrf = resp.Header.Get("x-csrf-token")
	}

	defer resp.Body.Close()

	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return byteArray, nil
}

// ListRecords retrieves all DNS records.
func (c *Client) ListRecords() ([]DNSRecord, error) {
	resp, err := c.GetData(UnifiDNSRecords)
	if err != nil {
		return nil, err
	}

	var records []DNSRecord
	err = json.Unmarshal(resp, &records)
	if err != nil {
		return nil, err
	}

	return records, nil
}

// CreateEndpoint creates a new DNS record.
func (c *Client) CreateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	record := DNSRecord{
		Enabled:    true,
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      endpoint.Targets[0],
	}

	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	resp, err := c.ShipData(UnifiDNSRecords, body)
	if err != nil {
		return nil, err
	}

	var newRecord DNSRecord
	err = json.Unmarshal(resp, &newRecord)
	if err != nil {
		return nil, err
	}

	return &newRecord, nil
}

// UpdateEndpoint updates an existing DNS record.
func (c *Client) UpdateEndpoint(endpoint *endpoint.Endpoint) (*DNSRecord, error) {
	id, err := c.LookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		return nil, err
	}

	record := DNSRecord{
		Key:        endpoint.DNSName,
		RecordType: endpoint.RecordType,
		TTL:        endpoint.RecordTTL,
		Value:      endpoint.Targets[0],
	}

	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	resp, err := c.PutData(fmt.Sprintf(UnifiDNSSelectRecord, c.BaseURL, id), body)
	if err != nil {
		return nil, err
	}

	var updatedRecord DNSRecord
	err = json.Unmarshal(resp, &updatedRecord)
	if err != nil {
		return nil, err
	}

	return &updatedRecord, nil
}

// DeleteEndpoint deletes a DNS record.
func (c *Client) DeleteEndpoint(endpoint *endpoint.Endpoint) error {
	id, err := c.LookupIdentifier(endpoint.DNSName, endpoint.RecordType)
	if err != nil {
		return err
	}

	_, err = c.DeleteData(fmt.Sprintf(UnifiDNSSelectRecord, c.BaseURL, id))
	if err != nil {
		return err
	}

	return nil
}

// LookupIdentifier finds the ID of a DNS record.
func (c *Client) LookupIdentifier(Key string, RecordType string) (string, error) {
	records, err := c.ListRecords()
	if err != nil {
		return "", err
	}

	for _, r := range records {
		if r.Key == Key && r.RecordType == RecordType {
			return r.ID, nil
		}
	}

	return "", fmt.Errorf("record not found")
}
