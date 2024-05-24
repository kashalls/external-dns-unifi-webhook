package dnsprovider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

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

// Client is the DNS provider client.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	csrf       string
}

var (
	UnifiLogin           = "%s/api/auth/login"
	UnifiDNSRecords      = "%s/proxy/network/v2/api/site/default/static-dns"
	UnifiDNSSelectRecord = "%s/proxy/network/v2/api/site/default/static-dns/%s"
)

// NewClient creates a new DNS provider client and logs in to store cookies.
func NewClient(baseURL, username, password string, skipTLSVerify bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify},
	}

	client := &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: transport,
			Jar:       jar,
		},
	}

	if err := client.login(username, password); err != nil {
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
	req, _ := http.NewRequest(http.MethodPost, url, nil)

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

// CreateRecord creates a new DNS record.
func (c *Client) CreateRecord(record DNSRecord) (*DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	log.Debugf("json marshal: %v", record)

	resp, err := c.ShipData(UnifiDNSRecords, body)
	if err != nil {
		return nil, err
	}

	log.Debugf("json marshal 2: %v", resp)

	var newRecord DNSRecord
	err = json.Unmarshal(resp, &newRecord)
	if err != nil {
		return nil, err
	}

	log.Debugf("json marshal 3: %v", newRecord)
	return &newRecord, nil
}

// UpdateRecord updates an existing DNS record.
func (c *Client) UpdateRecord(id string, record DNSRecord) (*DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	resp, err := c.PutData(fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns/%s", c.BaseURL, id), body)
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

// DeleteRecord deletes a DNS record.
func (c *Client) DeleteRecord(id string) error {
	_, err := c.DeleteData(fmt.Sprintf(UnifiDNSSelectRecord, c.BaseURL, id))
	if err != nil {
		return err
	}

	return nil
}

// DNSProvider implements the provider.Provider interface.
type DNSProvider struct {
	client *Client
}

// NewDNSProvider initializes a new DNSProvider.
func NewDNSProvider(baseURL, username, password string, skipTLSVerify bool) (provider.Provider, error) {
	client, err := NewClient(baseURL, username, password, skipTLSVerify)
	if err != nil {
		return nil, err
	}
	return &DNSProvider{
		client: client,
	}, nil
}

// Records returns the list of records in the DNS provider.
func (p *DNSProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.ListRecords()
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, record := range records {
		endpoints = append(endpoints, &endpoint.Endpoint{
			DNSName:       record.Key,
			Targets:       []string{record.Value},
			RecordType:    record.RecordType,
			SetIdentifier: record.ID,
			RecordTTL:     record.TTL,
		})
	}
	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *DNSProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	for _, ep := range changes.Create {
		record := DNSRecord{
			Key:        ep.DNSName,
			Value:      ep.Targets[0],
			RecordType: ep.RecordType,
			TTL:        ep.RecordTTL,
		}
		if _, err := p.client.CreateRecord(record); err != nil {
			return err
		}
	}

	for _, ep := range changes.UpdateNew {
		record := DNSRecord{
			ID:         ep.SetIdentifier,
			Key:        ep.DNSName,
			Value:      ep.Targets[0],
			RecordType: ep.RecordType,
			TTL:        ep.RecordTTL,
		}
		// Assuming ID can be obtained from DNS name
		id := ep.DNSName // This needs to be changed to actual ID fetching logic
		if _, err := p.client.UpdateRecord(id, record); err != nil {
			return err
		}
	}

	for _, ep := range changes.Delete {
		// Assuming ID can be obtained from DNS name
		id := ep.DNSName // This needs to be changed to actual ID fetching logic
		if err := p.client.DeleteRecord(id); err != nil {
			return err
		}
	}

	return nil
}

// AdjustEndpoints modifies the endpoints before they are sent to the DNS provider.
func (p *DNSProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	// Implement any adjustments needed to the endpoints before processing
	return endpoints, nil
}

// GetDomainFilter returns the domain filter for the provider.
func (p *DNSProvider) GetDomainFilter() endpoint.DomainFilter {
	// Since we're not using domain filtering, return an empty filter that matches everything
	return endpoint.NewDomainFilter([]string{})
}

// GetDNSName returns the DNS provider's name.
func (p *DNSProvider) GetDNSName() string {
	return "external-dns-unifi-webhook"
}
