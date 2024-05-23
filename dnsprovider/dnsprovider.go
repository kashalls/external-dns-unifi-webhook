package dnsprovider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// DNSRecord represents a DNS record in the API.
type DNSRecord struct {
	ID       string `json:"id,omitempty"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// Client is the DNS provider client.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

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
	loginURL := fmt.Sprintf("%s/api/auth/login", c.BaseURL)
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

// ListRecords retrieves all DNS records.
func (c *Client) ListRecords() ([]DNSRecord, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns", c.BaseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var records []DNSRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
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

	resp, err := c.HTTPClient.Post(fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns", c.BaseURL), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var newRecord DNSRecord
	if err := json.NewDecoder(resp.Body).Decode(&newRecord); err != nil {
		return nil, err
	}
	return &newRecord, nil
}

// UpdateRecord updates an existing DNS record.
func (c *Client) UpdateRecord(id string, record DNSRecord) (*DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns/%s", c.BaseURL, id), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var updatedRecord DNSRecord
	if err := json.NewDecoder(resp.Body).Decode(&updatedRecord); err != nil {
		return nil, err
	}
	return &updatedRecord, nil
}

// DeleteRecord deletes a DNS record.
func (c *Client) DeleteRecord(id string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns/%s", c.BaseURL, id), nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete record: %s", bodyBytes)
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
			DNSName:    record.Hostname,
			Targets:    endpoint.Targets{record.IP},
			RecordType: "A",
		})
	}
	return endpoints, nil
}

// ApplyChanges applies a given set of changes in the DNS provider.
func (p *DNSProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	for _, ep := range changes.Create {
		record := DNSRecord{
			Hostname: ep.DNSName,
			IP:       ep.Targets[0],
		}
		if _, err := p.client.CreateRecord(record); err != nil {
			return err
		}
	}

	for _, ep := range changes.UpdateNew {
		record := DNSRecord{
			Hostname: ep.DNSName,
			IP:       ep.Targets[0],
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
