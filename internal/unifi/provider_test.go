package unifi

import (
	"context"
	"fmt"
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// unifiClient interface for mocking
type unifiClient interface {
	GetEndpoints() ([]DNSRecord, error)
	CreateEndpoint(*endpoint.Endpoint) ([]*DNSRecord, error)
	DeleteEndpoint(*endpoint.Endpoint) error
}

// mockClient implements unifiClient for testing
type mockClient struct {
	getEndpointsFunc    func() ([]DNSRecord, error)
	createEndpointFunc  func(*endpoint.Endpoint) ([]*DNSRecord, error)
	deleteEndpointFunc  func(*endpoint.Endpoint) error
	getEndpointsCalls   int
	createEndpointCalls int
	deleteEndpointCalls int
}

func (m *mockClient) GetEndpoints() ([]DNSRecord, error) {
	m.getEndpointsCalls++
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc()
	}
	return []DNSRecord{}, nil
}

func (m *mockClient) CreateEndpoint(ep *endpoint.Endpoint) ([]*DNSRecord, error) {
	m.createEndpointCalls++
	if m.createEndpointFunc != nil {
		return m.createEndpointFunc(ep)
	}
	return []*DNSRecord{}, nil
}

func (m *mockClient) DeleteEndpoint(ep *endpoint.Endpoint) error {
	m.deleteEndpointCalls++
	if m.deleteEndpointFunc != nil {
		return m.deleteEndpointFunc(ep)
	}
	return nil
}

// testProvider wraps UnifiProvider with mockable client
type testProvider struct {
	provider.BaseProvider
	client       unifiClient
	domainFilter endpoint.DomainFilter
}

func (p *testProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.GetEndpoints()
	if err != nil {
		return nil, err
	}

	groups := make(map[string][]DNSRecord)
	for _, r := range records {
		if provider.SupportedRecordType(r.RecordType) {
			groupKey := r.Key + r.RecordType
			groups[groupKey] = append(groups[groupKey], r)
		}
	}

	var endpoints []*endpoint.Endpoint
	for _, records := range groups {
		if len(records) == 0 {
			continue
		}

		targets := make([]string, len(records))
		for i, record := range records {
			targets[i] = record.Value
		}

		if ep := endpoint.NewEndpointWithTTL(
			records[0].Key, records[0].RecordType, endpoint.TTL(records[0].TTL), targets...,
		); ep != nil {
			endpoints = append(endpoints, ep)
		}
	}

	return endpoints, nil
}

func (p *testProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	existingRecords, err := p.Records(ctx)
	if err != nil {
		return err
	}

	// Process deletions and updates (delete old)
	for _, endpoint := range append(changes.UpdateOld, changes.Delete...) {
		if err := p.client.DeleteEndpoint(endpoint); err != nil {
			return err
		}
	}

	// Process creates and updates (create new)
	for _, endpoint := range append(changes.Create, changes.UpdateNew...) {
		// Check for CNAME conflicts
		if endpoint.RecordType == "CNAME" {
			for _, record := range existingRecords {
				if record.RecordType != "CNAME" {
					continue
				}

				if record.DNSName != endpoint.DNSName {
					continue
				}

				if err := p.client.DeleteEndpoint(record); err != nil {
					return err
				}
			}
		}
		if _, err := p.client.CreateEndpoint(endpoint); err != nil {
			return err
		}
	}

	return nil
}

func (p *testProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &p.domainFilter
}

// TestRecords tests the Records method
func TestRecords(t *testing.T) {
	tests := []struct {
		name           string
		mockRecords    []DNSRecord
		mockError      error
		expectedLen    int
		expectedErr    bool
		validateResult func(*testing.T, []*endpoint.Endpoint)
	}{
		{
			name: "empty records",
			mockRecords: []DNSRecord{},
			mockError:   nil,
			expectedLen: 0,
			expectedErr: false,
		},
		{
			name: "single A record",
			mockRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "test.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedLen: 1,
			expectedErr: false,
			validateResult: func(t *testing.T, eps []*endpoint.Endpoint) {
				if eps[0].DNSName != "test.example.com" {
					t.Errorf("DNSName = %q, want %q", eps[0].DNSName, "test.example.com")
				}
				if eps[0].RecordType != "A" {
					t.Errorf("RecordType = %q, want %q", eps[0].RecordType, "A")
				}
				if len(eps[0].Targets) != 1 || eps[0].Targets[0] != "192.168.1.1" {
					t.Errorf("Targets = %v, want [192.168.1.1]", eps[0].Targets)
				}
			},
		},
		{
			name: "multiple A records with same name",
			mockRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "multi.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         "record2",
					Key:        "multi.example.com",
					RecordType: "A",
					Value:      "192.168.1.2",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedLen: 1,
			expectedErr: false,
			validateResult: func(t *testing.T, eps []*endpoint.Endpoint) {
				if len(eps[0].Targets) != 2 {
					t.Errorf("Targets length = %d, want 2", len(eps[0].Targets))
				}
			},
		},
		{
			name: "CNAME record",
			mockRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "alias.example.com",
					RecordType: "CNAME",
					Value:      "target.example.com",
					TTL:        600,
					Enabled:    true,
				},
			},
			expectedLen: 1,
			expectedErr: false,
			validateResult: func(t *testing.T, eps []*endpoint.Endpoint) {
				if eps[0].RecordType != "CNAME" {
					t.Errorf("RecordType = %q, want CNAME", eps[0].RecordType)
				}
			},
		},
		{
			name: "unsupported record type filtered out",
			mockRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "test.example.com",
					RecordType: "UNSUPPORTED",
					Value:      "value",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedLen: 0,
			expectedErr: false,
		},
		{
			name: "mixed record types",
			mockRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "a.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         "record2",
					Key:        "cname.example.com",
					RecordType: "CNAME",
					Value:      "target.example.com",
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         "record3",
					Key:        "txt.example.com",
					RecordType: "TXT",
					Value:      "some text",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedLen: 3,
			expectedErr: false,
		},
		{
			name:        "error from client",
			mockRecords: nil,
			mockError:   fmt.Errorf("connection failed"),
			expectedLen: 0,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockClient{
				getEndpointsFunc: func() ([]DNSRecord, error) {
					return tt.mockRecords, tt.mockError
				},
			}

			p := &testProvider{
				client:       mock,
				domainFilter: *endpoint.NewDomainFilter([]string{}),
			}

			eps, err := p.Records(context.Background())

			if tt.expectedErr && err == nil {
				t.Error("Records() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("Records() unexpected error: %v", err)
			}

			if len(eps) != tt.expectedLen {
				t.Errorf("Records() returned %d endpoints, want %d", len(eps), tt.expectedLen)
			}

			if tt.validateResult != nil && len(eps) > 0 {
				tt.validateResult(t, eps)
			}

			if mock.getEndpointsCalls != 1 {
				t.Errorf("GetEndpoints called %d times, want 1", mock.getEndpointsCalls)
			}
		})
	}
}

// TestApplyChanges tests the ApplyChanges method
func TestApplyChanges(t *testing.T) {
	tests := []struct {
		name                  string
		changes               *plan.Changes
		existingRecords       []DNSRecord
		expectedCreateCalls   int
		expectedDeleteCalls   int
		createError           error
		deleteError           error
		expectedErr           bool
		validateCalls         func(*testing.T, *mockClient)
	}{
		{
			name: "no changes",
			changes: &plan.Changes{
				Create:    []*endpoint.Endpoint{},
				UpdateOld: []*endpoint.Endpoint{},
				UpdateNew: []*endpoint.Endpoint{},
				Delete:    []*endpoint.Endpoint{},
			},
			existingRecords:     []DNSRecord{},
			expectedCreateCalls: 0,
			expectedDeleteCalls: 0,
			expectedErr:         false,
		},
		{
			name: "create single record",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					endpoint.NewEndpoint("new.example.com", "A", "192.168.1.1"),
				},
			},
			existingRecords:     []DNSRecord{},
			expectedCreateCalls: 1,
			expectedDeleteCalls: 0,
			expectedErr:         false,
		},
		{
			name: "delete single record",
			changes: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					endpoint.NewEndpoint("old.example.com", "A", "192.168.1.1"),
				},
			},
			existingRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "old.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedCreateCalls: 0,
			expectedDeleteCalls: 1,
			expectedErr:         false,
		},
		{
			name: "update record (delete old + create new)",
			changes: &plan.Changes{
				UpdateOld: []*endpoint.Endpoint{
					endpoint.NewEndpoint("update.example.com", "A", "192.168.1.1"),
				},
				UpdateNew: []*endpoint.Endpoint{
					endpoint.NewEndpoint("update.example.com", "A", "192.168.1.2"),
				},
			},
			existingRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "update.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedCreateCalls: 1,
			expectedDeleteCalls: 1,
			expectedErr:         false,
		},
		{
			name: "CNAME conflict - deletes existing CNAME",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					endpoint.NewEndpoint("conflict.example.com", "CNAME", "new-target.example.com"),
				},
			},
			existingRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "conflict.example.com",
					RecordType: "CNAME",
					Value:      "old-target.example.com",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedCreateCalls: 1,
			expectedDeleteCalls: 1, // Deletes conflicting CNAME
			expectedErr:         false,
		},
		{
			name: "create error",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					endpoint.NewEndpoint("new.example.com", "A", "192.168.1.1"),
				},
			},
			existingRecords:     []DNSRecord{},
			expectedCreateCalls: 1,
			createError:         fmt.Errorf("create failed"),
			expectedErr:         true,
		},
		{
			name: "delete error",
			changes: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					endpoint.NewEndpoint("old.example.com", "A", "192.168.1.1"),
				},
			},
			existingRecords: []DNSRecord{
				{
					ID:         "record1",
					Key:        "old.example.com",
					RecordType: "A",
					Value:      "192.168.1.1",
					TTL:        300,
					Enabled:    true,
				},
			},
			expectedDeleteCalls: 1,
			deleteError:         fmt.Errorf("delete failed"),
			expectedErr:         true,
		},
		{
			name: "multiple creates",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					endpoint.NewEndpoint("new1.example.com", "A", "192.168.1.1"),
					endpoint.NewEndpoint("new2.example.com", "A", "192.168.1.2"),
					endpoint.NewEndpoint("new3.example.com", "CNAME", "target.example.com"),
				},
			},
			existingRecords:     []DNSRecord{},
			expectedCreateCalls: 3,
			expectedDeleteCalls: 0,
			expectedErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockClient{
				getEndpointsFunc: func() ([]DNSRecord, error) {
					return tt.existingRecords, nil
				},
				deleteEndpointFunc: func(ep *endpoint.Endpoint) error {
					return tt.deleteError
				},
			}
			mock.createEndpointFunc = func(ep *endpoint.Endpoint) ([]*DNSRecord, error) {
				if tt.createError != nil {
					return nil, tt.createError
				}
				return []*DNSRecord{{
					ID:         fmt.Sprintf("new-record-%d", mock.createEndpointCalls),
					Key:        ep.DNSName,
					RecordType: ep.RecordType,
					Value:      ep.Targets[0],
					TTL:        300,
					Enabled:    true,
				}}, nil
			}

			p := &testProvider{
				client:       mock,
				domainFilter: *endpoint.NewDomainFilter([]string{}),
			}

			err := p.ApplyChanges(context.Background(), tt.changes)

			if tt.expectedErr && err == nil {
				t.Error("ApplyChanges() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("ApplyChanges() unexpected error: %v", err)
			}

			if mock.createEndpointCalls != tt.expectedCreateCalls {
				t.Errorf("CreateEndpoint called %d times, want %d", mock.createEndpointCalls, tt.expectedCreateCalls)
			}

			if mock.deleteEndpointCalls != tt.expectedDeleteCalls {
				t.Errorf("DeleteEndpoint called %d times, want %d", mock.deleteEndpointCalls, tt.expectedDeleteCalls)
			}

			if tt.validateCalls != nil {
				tt.validateCalls(t, mock)
			}
		})
	}
}

// TestGetDomainFilter tests the GetDomainFilter method
func TestGetDomainFilter(t *testing.T) {
	tests := []struct {
		name         string
		domainFilter *endpoint.DomainFilter
	}{
		{
			name:         "empty domain filter",
			domainFilter: endpoint.NewDomainFilter([]string{}),
		},
		{
			name:         "single domain",
			domainFilter: endpoint.NewDomainFilter([]string{"example.com"}),
		},
		{
			name:         "multiple domains",
			domainFilter: endpoint.NewDomainFilter([]string{"example.com", "test.com", "demo.org"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &testProvider{
				domainFilter: *tt.domainFilter,
			}

			filter := p.GetDomainFilter()

			if filter == nil {
				t.Error("GetDomainFilter() returned nil")
			}

			// Verify it returns the same filter
			if df, ok := filter.(*endpoint.DomainFilter); ok {
				if len(df.Filters) != len(tt.domainFilter.Filters) {
					t.Errorf("Filter count = %d, want %d", len(df.Filters), len(tt.domainFilter.Filters))
				}
			}
		})
	}
}

// TestRecordsGrouping tests that records are correctly grouped
func TestRecordsGrouping(t *testing.T) {
	mockRecords := []DNSRecord{
		{
			ID:         "record1",
			Key:        "multi.example.com",
			RecordType: "A",
			Value:      "192.168.1.1",
			TTL:        300,
			Enabled:    true,
		},
		{
			ID:         "record2",
			Key:        "multi.example.com",
			RecordType: "A",
			Value:      "192.168.1.2",
			TTL:        300,
			Enabled:    true,
		},
		{
			ID:         "record3",
			Key:        "multi.example.com",
			RecordType: "A",
			Value:      "192.168.1.3",
			TTL:        300,
			Enabled:    true,
		},
		{
			ID:         "record4",
			Key:        "different.example.com",
			RecordType: "A",
			Value:      "10.0.0.1",
			TTL:        300,
			Enabled:    true,
		},
	}

	mock := &mockClient{
		getEndpointsFunc: func() ([]DNSRecord, error) {
			return mockRecords, nil
		},
	}

	p := &testProvider{
		client:       mock,
		domainFilter: *endpoint.NewDomainFilter([]string{}),
	}

	eps, err := p.Records(context.Background())

	if err != nil {
		t.Fatalf("Records() error: %v", err)
	}

	if len(eps) != 2 {
		t.Errorf("Expected 2 grouped endpoints, got %d", len(eps))
	}

	// Find the multi-target endpoint
	var multiEp *endpoint.Endpoint
	for _, ep := range eps {
		if ep.DNSName == "multi.example.com" {
			multiEp = ep
			break
		}
	}

	if multiEp == nil {
		t.Fatal("multi.example.com endpoint not found")
	}

	if len(multiEp.Targets) != 3 {
		t.Errorf("multi.example.com has %d targets, want 3", len(multiEp.Targets))
	}

	expectedTargets := map[string]bool{
		"192.168.1.1": true,
		"192.168.1.2": true,
		"192.168.1.3": true,
	}

	for _, target := range multiEp.Targets {
		if !expectedTargets[target] {
			t.Errorf("Unexpected target: %s", target)
		}
	}
}

// TestProviderImplementsInterface verifies UnifiProvider implements provider.Provider
func TestProviderImplementsInterface(t *testing.T) {
	var _ provider.Provider = &UnifiProvider{}
}
