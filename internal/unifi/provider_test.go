package unifi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

const (
	testExampleDomain = "test.example.com"
)

var (
	errAPIFailure    = errors.New("API error")
	errCreateFailure = errors.New("create failed")
	errDeleteFailure = errors.New("delete failed")
)

// mockUnifiAPI is a mock implementation of UnifiAPI for testing.
type mockUnifiAPI struct {
	getEndpointsFunc   func(ctx context.Context) ([]DNSRecord, error)
	createEndpointFunc func(ctx context.Context, endpoint *endpoint.Endpoint) ([]*DNSRecord, error)
	deleteEndpointFunc func(ctx context.Context, endpoint *endpoint.Endpoint) error
}

func (m *mockUnifiAPI) GetEndpoints(ctx context.Context) ([]DNSRecord, error) {
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc(ctx)
	}

	return nil, nil
}

func (m *mockUnifiAPI) CreateEndpoint(ctx context.Context, ep *endpoint.Endpoint) ([]*DNSRecord, error) {
	if m.createEndpointFunc != nil {
		return m.createEndpointFunc(ctx, ep)
	}

	return nil, nil
}

func (m *mockUnifiAPI) DeleteEndpoint(ctx context.Context, ep *endpoint.Endpoint) error {
	if m.deleteEndpointFunc != nil {
		return m.deleteEndpointFunc(ctx, ep)
	}

	return nil
}

// mockMetricsRecorder is a mock implementation of MetricsRecorder for testing.
type mockMetricsRecorder struct {
	recordUniFiAPICallFunc   func(operation string, duration time.Duration, responseSize int, err error)
	recordChangeFunc         func(operation, recordType string)
	updateRecordsByTypeFunc  func(recordType string, count int)
	ignoredCNAMETargetsTotal prometheus.Counter
	srvParsingErrorsTotal    prometheus.Counter
	cnameConflictsTotal      *prometheus.CounterVec
	batchSize                *prometheus.HistogramVec
	unifiLoginTotal          *prometheus.CounterVec
	unifiConnected           *prometheus.GaugeVec
	unifiCSRFRefreshesTotal  *prometheus.CounterVec
	unifiReloginTotal        *prometheus.CounterVec
}

func (m *mockMetricsRecorder) RecordUniFiAPICall(operation string, duration time.Duration, responseSize int, err error) {
	if m.recordUniFiAPICallFunc != nil {
		m.recordUniFiAPICallFunc(operation, duration, responseSize, err)
	}
}

func (m *mockMetricsRecorder) RecordChange(operation, recordType string) {
	if m.recordChangeFunc != nil {
		m.recordChangeFunc(operation, recordType)
	}
}

func (m *mockMetricsRecorder) UpdateRecordsByType(recordType string, count int) {
	if m.updateRecordsByTypeFunc != nil {
		m.updateRecordsByTypeFunc(recordType, count)
	}
}

func (m *mockMetricsRecorder) IgnoredCNAMETargetsTotal() prometheus.Counter {
	if m.ignoredCNAMETargetsTotal != nil {
		return m.ignoredCNAMETargetsTotal
	}

	return prometheus.NewCounter(prometheus.CounterOpts{Name: "ignored_cname_targets_total", Help: "Total number of ignored CNAME targets"})
}

func (m *mockMetricsRecorder) SRVParsingErrorsTotal() prometheus.Counter {
	if m.srvParsingErrorsTotal != nil {
		return m.srvParsingErrorsTotal
	}

	return prometheus.NewCounter(prometheus.CounterOpts{Name: "srv_parsing_errors_total", Help: "Total number of SRV parsing errors"})
}

func (m *mockMetricsRecorder) CNAMEConflictsTotal() *prometheus.CounterVec {
	return m.cnameConflictsTotal
}

func (m *mockMetricsRecorder) BatchSize() *prometheus.HistogramVec {
	return m.batchSize
}

func (m *mockMetricsRecorder) UniFiLoginTotal() *prometheus.CounterVec {
	return m.unifiLoginTotal
}

func (m *mockMetricsRecorder) UniFiConnected() *prometheus.GaugeVec {
	return m.unifiConnected
}

func (m *mockMetricsRecorder) UniFiCSRFRefreshesTotal() *prometheus.CounterVec {
	return m.unifiCSRFRefreshesTotal
}

func (m *mockMetricsRecorder) UniFiReloginTotal() *prometheus.CounterVec {
	return m.unifiReloginTotal
}

// mockLogger is a mock implementation of Logger for testing.
type mockLogger struct {
	debugCalls []logCall
	infoCalls  []logCall
	warnCalls  []logCall
	errorCalls []logCall
}

type logCall struct {
	msg           string
	keysAndValues []any
}

func (m *mockLogger) Debug(msg string, keysAndValues ...any) {
	m.debugCalls = append(m.debugCalls, logCall{msg: msg, keysAndValues: keysAndValues})
}

func (m *mockLogger) Info(msg string, keysAndValues ...any) {
	m.infoCalls = append(m.infoCalls, logCall{msg: msg, keysAndValues: keysAndValues})
}

func (m *mockLogger) Warn(msg string, keysAndValues ...any) {
	m.warnCalls = append(m.warnCalls, logCall{msg: msg, keysAndValues: keysAndValues})
}

func (m *mockLogger) Error(msg string, keysAndValues ...any) {
	m.errorCalls = append(m.errorCalls, logCall{msg: msg, keysAndValues: keysAndValues})
}

func TestUnifiProvider_Records(t *testing.T) {
	tests := []struct {
		name           string
		mockRecords    []DNSRecord
		mockError      error
		expectedLen    int
		expectedErr    bool
		validateResult func(*testing.T, []*endpoint.Endpoint)
	}{
		{
			name: "successful fetch with A records",
			mockRecords: []DNSRecord{
				{ID: "1", Key: "test.example.com", RecordType: "A", Value: "192.168.1.1", TTL: 300},
				{ID: "2", Key: "test.example.com", RecordType: "A", Value: "192.168.1.2", TTL: 300},
			},
			expectedLen: 1,
			expectedErr: false,
			validateResult: func(t *testing.T, endpoints []*endpoint.Endpoint) {
				t.Helper()
				if endpoints[0].DNSName != testExampleDomain {
					t.Errorf("DNSName = %q, want test.example.com", endpoints[0].DNSName)
				}
				if len(endpoints[0].Targets) != 2 {
					t.Errorf("Targets length = %d, want 2", len(endpoints[0].Targets))
				}
			},
		},
		{
			name: "mixed record types",
			mockRecords: []DNSRecord{
				{ID: "1", Key: "a.example.com", RecordType: "A", Value: "1.2.3.4", TTL: 300},
				{ID: "2", Key: "cname.example.com", RecordType: "CNAME", Value: "target.example.com", TTL: 300},
				{ID: "3", Key: "txt.example.com", RecordType: "TXT", Value: "v=spf1", TTL: 300},
			},
			expectedLen: 3,
			expectedErr: false,
		},
		{
			name:        "API error",
			mockError:   errAPIFailure,
			expectedLen: 0,
			expectedErr: true,
		},
		{
			name:        "empty response",
			mockRecords: []DNSRecord{},
			expectedLen: 0,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockUnifiAPI{
				getEndpointsFunc: func(_ context.Context) ([]DNSRecord, error) {
					return tt.mockRecords, tt.mockError
				},
			}

			mockMetrics := &mockMetricsRecorder{
				updateRecordsByTypeFunc: func(_ string, _ int) {},
			}

			mockLog := &mockLogger{}

			provider := &UnifiProvider{
				api:          mockAPI,
				domainFilter: endpoint.DomainFilter{},
				metrics:      mockMetrics,
				logger:       mockLog,
			}

			endpoints, err := provider.Records(context.Background())

			if tt.expectedErr && err == nil {
				t.Error("Records() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("Records() unexpected error: %v", err)
			}

			if len(endpoints) != tt.expectedLen {
				t.Errorf("Records() returned %d endpoints, want %d", len(endpoints), tt.expectedLen)
			}

			if tt.validateResult != nil && !tt.expectedErr {
				tt.validateResult(t, endpoints)
			}
		})
	}
}

func TestUnifiProvider_ApplyChanges(t *testing.T) {
	tests := []struct {
		name               string
		changes            *plan.Changes
		existingRecords    []DNSRecord
		createError        error
		deleteError        error
		expectedErr        bool
		expectedCreateCall int
		expectedDeleteCall int
	}{
		{
			name: "create new A record",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{DNSName: "new.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}, RecordTTL: 300},
				},
			},
			existingRecords:    []DNSRecord{},
			expectedErr:        false,
			expectedCreateCall: 1,
			expectedDeleteCall: 0,
		},
		{
			name: "delete existing record",
			changes: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{DNSName: "old.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}, RecordTTL: 300},
				},
			},
			existingRecords: []DNSRecord{
				{ID: "1", Key: "old.example.com", RecordType: "A", Value: "1.2.3.4", TTL: 300},
			},
			expectedErr:        false,
			expectedCreateCall: 0,
			expectedDeleteCall: 1,
		},
		{
			name: "update record (delete old + create new)",
			changes: &plan.Changes{
				UpdateOld: []*endpoint.Endpoint{
					{DNSName: "update.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}, RecordTTL: 300},
				},
				UpdateNew: []*endpoint.Endpoint{
					{DNSName: "update.example.com", RecordType: "A", Targets: []string{"5.6.7.8"}, RecordTTL: 300},
				},
			},
			existingRecords: []DNSRecord{
				{ID: "1", Key: "update.example.com", RecordType: "A", Value: "1.2.3.4", TTL: 300},
			},
			expectedErr:        false,
			expectedCreateCall: 1,
			expectedDeleteCall: 1,
		},
		{
			name: "CNAME conflict detection",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{DNSName: "cname.example.com", RecordType: "CNAME", Targets: []string{"target.example.com"}, RecordTTL: 300},
				},
			},
			existingRecords: []DNSRecord{
				{ID: "1", Key: "cname.example.com", RecordType: "CNAME", Value: "old-target.example.com", TTL: 300},
			},
			expectedErr:        false,
			expectedCreateCall: 1,
			expectedDeleteCall: 1, // Deletes conflicting CNAME
		},
		{
			name: "create error handling",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{DNSName: "new.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}, RecordTTL: 300},
				},
			},
			existingRecords:    []DNSRecord{},
			createError:        errCreateFailure,
			expectedErr:        true,
			expectedCreateCall: 1,
			expectedDeleteCall: 0,
		},
		{
			name: "delete error handling",
			changes: &plan.Changes{
				Delete: []*endpoint.Endpoint{
					{DNSName: "old.example.com", RecordType: "A", Targets: []string{"1.2.3.4"}, RecordTTL: 300},
				},
			},
			existingRecords: []DNSRecord{
				{ID: "1", Key: "old.example.com", RecordType: "A", Value: "1.2.3.4", TTL: 300},
			},
			deleteError:        errDeleteFailure,
			expectedErr:        true,
			expectedCreateCall: 0,
			expectedDeleteCall: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createCallCount := 0
			deleteCallCount := 0

			mockAPI := &mockUnifiAPI{
				getEndpointsFunc: func(_ context.Context) ([]DNSRecord, error) {
					return tt.existingRecords, nil
				},
				createEndpointFunc: func(_ context.Context, endpointToCreate *endpoint.Endpoint) ([]*DNSRecord, error) {
					createCallCount++
					if tt.createError != nil {
						return nil, tt.createError
					}

					return []*DNSRecord{{ID: "new", Key: endpointToCreate.DNSName, RecordType: endpointToCreate.RecordType}}, nil
				},
				deleteEndpointFunc: func(_ context.Context, _ *endpoint.Endpoint) error {
					deleteCallCount++

					return tt.deleteError
				},
			}

			mockMetrics := &mockMetricsRecorder{
				recordChangeFunc:        func(_, _ string) {},
				updateRecordsByTypeFunc: func(_ string, _ int) {},
				batchSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
					Name: "test_batch_size",
					Help: "Batch size for operations",
				}, []string{"provider", "operation"}),
				cnameConflictsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
					Name: "test_cname_conflicts_total",
					Help: "Total number of CNAME conflicts",
				}, []string{"provider"}),
			}

			mockLog := &mockLogger{}

			provider := &UnifiProvider{
				api:          mockAPI,
				domainFilter: endpoint.DomainFilter{},
				metrics:      mockMetrics,
				logger:       mockLog,
			}

			err := provider.ApplyChanges(context.Background(), tt.changes)

			if tt.expectedErr && err == nil {
				t.Error("ApplyChanges() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("ApplyChanges() unexpected error: %v", err)
			}

			if createCallCount != tt.expectedCreateCall {
				t.Errorf("CreateEndpoint called %d times, want %d", createCallCount, tt.expectedCreateCall)
			}

			if deleteCallCount != tt.expectedDeleteCall {
				t.Errorf("DeleteEndpoint called %d times, want %d", deleteCallCount, tt.expectedDeleteCall)
			}
		})
	}
}

func TestUnifiProvider_GetDomainFilter(t *testing.T) {
	domainFilter := endpoint.NewDomainFilter([]string{"example.com"})

	provider := &UnifiProvider{
		domainFilter: *domainFilter,
	}

	result := provider.GetDomainFilter()

	if result == nil {
		t.Error("GetDomainFilter() returned nil")
	}
}

func TestNewUnifiProvider_Success(t *testing.T) {
	mockAPI := &mockUnifiAPI{}
	mockMetrics := &mockMetricsRecorder{}
	mockLog := &mockLogger{}
	domainFilter := endpoint.DomainFilter{}

	provider, err := NewUnifiProvider(mockAPI, domainFilter, mockMetrics, mockLog)
	if err != nil {
		t.Fatalf("NewUnifiProvider() unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatal("NewUnifiProvider() returned nil provider")
	}

	unifiProvider, ok := provider.(*UnifiProvider)
	if !ok {
		t.Fatal("NewUnifiProvider() did not return *UnifiProvider")
	}

	if unifiProvider.api != mockAPI {
		t.Error("NewUnifiProvider() did not set api correctly")
	}

	if unifiProvider.metrics != mockMetrics {
		t.Error("NewUnifiProvider() did not set metrics correctly")
	}

	if unifiProvider.logger != mockLog {
		t.Error("NewUnifiProvider() did not set logger correctly")
	}
}

func TestNewUnifiProvider_WithDomainFilter(t *testing.T) {
	mockAPI := &mockUnifiAPI{}
	mockMetrics := &mockMetricsRecorder{}
	mockLog := &mockLogger{}
	domainFilter := *endpoint.NewDomainFilter([]string{"example.com", "test.com"})

	provider, err := NewUnifiProvider(mockAPI, domainFilter, mockMetrics, mockLog)
	if err != nil {
		t.Fatalf("NewUnifiProvider() unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatal("NewUnifiProvider() returned nil provider")
	}

	unifiProvider, ok := provider.(*UnifiProvider)
	if !ok {
		t.Fatal("Provider is not *UnifiProvider")
	}

	result := unifiProvider.GetDomainFilter()
	if result == nil {
		t.Fatal("Domain filter is nil")
	}
}

func TestNewUnifiProviderFromConfig_Success(t *testing.T) {
	config := &Config{
		Host:          "https://unifi.example.com",
		APIKey:        "test-api-key",
		Site:          "default",
		SkipTLSVerify: true,
	}

	domainFilter := endpoint.DomainFilter{}

	provider, err := NewUnifiProviderFromConfig(domainFilter, config)
	if err != nil {
		t.Fatalf("NewUnifiProviderFromConfig() unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatal("NewUnifiProviderFromConfig() returned nil provider")
	}

	unifiProvider, ok := provider.(*UnifiProvider)
	if !ok {
		t.Fatal("NewUnifiProviderFromConfig() did not return *UnifiProvider")
	}

	if unifiProvider.api == nil {
		t.Error("NewUnifiProviderFromConfig() did not set api")
	}

	if unifiProvider.metrics == nil {
		t.Error("NewUnifiProviderFromConfig() did not set metrics")
	}

	if unifiProvider.logger == nil {
		t.Error("NewUnifiProviderFromConfig() did not set logger")
	}
}

func TestNewUnifiProviderFromConfig_WithDomainFilter(t *testing.T) {
	config := &Config{
		Host:          "https://unifi.example.com",
		APIKey:        "test-api-key",
		Site:          "default",
		SkipTLSVerify: true,
	}

	domainFilter := *endpoint.NewDomainFilter([]string{"example.com"})

	provider, err := NewUnifiProviderFromConfig(domainFilter, config)
	if err != nil {
		t.Fatalf("NewUnifiProviderFromConfig() unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatal("NewUnifiProviderFromConfig() returned nil provider")
	}

	unifiProvider, ok := provider.(*UnifiProvider)
	if !ok {
		t.Fatal("Provider is not *UnifiProvider")
	}

	result := unifiProvider.GetDomainFilter()
	if result == nil {
		t.Error("Domain filter is nil")
	}
}
