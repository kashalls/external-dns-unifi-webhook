package unifi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"sigs.k8s.io/external-dns/endpoint"
)

func init() {
	// Initialize logger for tests
	log.Init()
	// metrics.Get() will initialize on first use
	_ = metrics.Get()
}

const (
	testDomain = "test.example.com"
)

// TestGetEndpoints tests the GetEndpoints method with mock HTTP server.
func TestGetEndpoints(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   any
		responseStatus int
		expectedLen    int
		expectedErr    bool
		validateResult func(*testing.T, []DNSRecord)
	}{
		{
			name: "successful fetch with A records",
			responseBody: []DNSRecord{
				{
					ID:         testDNSID1,
					Key:        testDomain,
					RecordType: recordTypeA,
					Value:      testIPv4A,
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         testDNSID2,
					Key:        "test2.example.com",
					RecordType: recordTypeA,
					Value:      testIPv4B,
					TTL:        600,
					Enabled:    true,
				},
			},
			responseStatus: http.StatusOK,
			expectedLen:    2,
			expectedErr:    false,
			validateResult: func(t *testing.T, records []DNSRecord) {
				t.Helper()
				if records[0].Key != testDomain {
					t.Errorf("First record Key = %q, want %q", records[0].Key, testDomain)
				}
				if records[0].RecordType != recordTypeA {
					t.Errorf("First record RecordType = %q, want A", records[0].RecordType)
				}
			},
		},
		{
			name: "SRV record transformation",
			responseBody: []DNSRecord{
				{
					ID:         "srv1",
					Key:        testSRVName,
					RecordType: recordTypeSRV,
					Value:      testTarget,
					TTL:        300,
					Priority:   intPtr(10),
					Weight:     intPtr(20),
					Port:       intPtr(8080),
					Enabled:    true,
				},
			},
			responseStatus: http.StatusOK,
			expectedLen:    1,
			expectedErr:    false,
			validateResult: func(t *testing.T, records []DNSRecord) {
				t.Helper()
				expected := "10 20 8080 target.example.com"
				if records[0].Value != expected {
					t.Errorf("SRV Value = %q, want %q", records[0].Value, expected)
				}
				if records[0].Priority != nil {
					t.Error("SRV Priority should be nil after transformation")
				}
				if records[0].Weight != nil {
					t.Error("SRV Weight should be nil after transformation")
				}
				if records[0].Port != nil {
					t.Error("SRV Port should be nil after transformation")
				}
			},
		},
		{
			name:           "empty response",
			responseBody:   []DNSRecord{},
			responseStatus: http.StatusOK,
			expectedLen:    0,
			expectedErr:    false,
		},
		{
			name:           "invalid JSON response",
			responseBody:   `{"invalid": json}`,
			responseStatus: http.StatusOK,
			expectedLen:    0,
			expectedErr:    true,
		},
		{
			name:           "HTTP 500 error",
			responseBody:   UnifiErrorResponse{Code: testErrCode, Message: testMsgInternalServer},
			responseStatus: http.StatusInternalServerError,
			expectedLen:    0,
			expectedErr:    true,
		},
		{
			name: "mixed record types",
			responseBody: []DNSRecord{
				{
					ID:         "a1",
					Key:        "a.example.com",
					RecordType: recordTypeA,
					Value:      "1.2.3.4",
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         testCNAMEID,
					Key:        "cname.example.com",
					RecordType: recordTypeCNAME,
					Value:      testTarget,
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         "txt1",
					Key:        "txt.example.com",
					RecordType: recordTypeTXT,
					Value:      "v=spf1 include:example.com ~all",
					TTL:        300,
					Enabled:    true,
				},
			},
			responseStatus: http.StatusOK,
			expectedLen:    3,
			expectedErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.responseStatus)
				if tt.responseStatus == http.StatusOK {
					if strBody, ok := tt.responseBody.(string); ok {
						_, _ = w.Write([]byte(strBody))
					} else {
						_ = json.NewEncoder(w).Encode(tt.responseBody)
					}
				} else {
					_ = json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			client := &httpClient{
				Config: &Config{
					Host:          server.URL,
					Site:          testSite,
					APIKey:        testKeyA,
					SkipTLSVerify: true,
				},
				Client: server.Client(),
				ClientURLs: &ClientURLs{
					Records: unifiRecordPathExternal,
				},
			}

			records, err := client.GetEndpoints(context.Background())

			if tt.expectedErr && err == nil {
				t.Error("GetEndpoints() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("GetEndpoints() unexpected error: %v", err)
			}

			if len(records) != tt.expectedLen {
				t.Errorf("GetEndpoints() returned %d records, want %d", len(records), tt.expectedLen)
			}

			if tt.validateResult != nil && len(records) > 0 {
				tt.validateResult(t, records)
			}
		})
	}
}

// TestCreateEndpoint tests the CreateEndpoint method.
func TestCreateEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       *endpoint.Endpoint
		responseBody   any
		responseStatus int
		expectedErr    bool
		validateReq    func(*testing.T, []byte)
	}{
		{
			name: "create A record",
			endpoint: endpoint.NewEndpointWithTTL(
				testDomain,
				"A",
				300,
				testIPv4A,
			),
			responseBody: DNSRecord{
				ID:         "new-record-1",
				Key:        testDomain,
				RecordType: recordTypeA,
				Value:      testIPv4A,
				TTL:        300,
				Enabled:    true,
			},
			responseStatus: http.StatusOK,
			expectedErr:    false,
			validateReq: func(t *testing.T, bodyBytes []byte) {
				t.Helper()
				var record DNSRecord
				err := json.Unmarshal(bodyBytes, &record)
				if err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}
				if record.Key != testDomain {
					t.Errorf("Request Key = %q, want test.example.com", record.Key)
				}
				if record.RecordType != recordTypeA {
					t.Errorf("Request RecordType = %q, want A", record.RecordType)
				}
				if record.Value != testIPv4A {
					t.Errorf("Request Value = %q, want 192.168.1.1", record.Value)
				}
			},
		},
		{
			name: "create CNAME record",
			endpoint: endpoint.NewEndpointWithTTL(
				testAlias,
				"CNAME",
				600,
				testTarget,
			),
			responseBody: DNSRecord{
				ID:         "new-cname-1",
				Key:        testAlias,
				RecordType: recordTypeCNAME,
				Value:      testTarget,
				TTL:        600,
				Enabled:    true,
			},
			responseStatus: http.StatusOK,
			expectedErr:    false,
		},
		{
			name: "create SRV record",
			endpoint: endpoint.NewEndpointWithTTL(
				testSRVName,
				"SRV",
				300,
				"10 20 8080 target.example.com",
			),
			responseBody: DNSRecord{
				ID:         "new-srv-1",
				Key:        testSRVName,
				RecordType: recordTypeSRV,
				Value:      testTarget,
				TTL:        300,
				Priority:   intPtr(10),
				Weight:     intPtr(20),
				Port:       intPtr(8080),
				Enabled:    true,
			},
			responseStatus: http.StatusOK,
			expectedErr:    false,
			validateReq: func(t *testing.T, bodyBytes []byte) {
				t.Helper()
				var record DNSRecord
				err := json.Unmarshal(bodyBytes, &record)
				if err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}
				if record.Priority == nil || *record.Priority != 10 {
					t.Errorf("SRV Priority = %v, want 10", record.Priority)
				}
				if record.Weight == nil || *record.Weight != 20 {
					t.Errorf("SRV Weight = %v, want 20", record.Weight)
				}
				if record.Port == nil || *record.Port != 8080 {
					t.Errorf("SRV Port = %v, want 8080", record.Port)
				}
			},
		},
		{
			name: "create multiple CNAME targets - uses only first",
			endpoint: endpoint.NewEndpointWithTTL(
				testMultiName,
				"CNAME",
				300,
				"target1.example.com",
				"target2.example.com",
			),
			responseBody: DNSRecord{
				ID:         "new-record",
				Key:        testMultiName,
				RecordType: recordTypeCNAME,
				Value:      "target1.example.com",
				TTL:        300,
				Enabled:    true,
			},
			responseStatus: http.StatusOK,
			expectedErr:    false,
		},
		{
			name: "HTTP 400 error",
			endpoint: endpoint.NewEndpointWithTTL(
				testDomain,
				"A",
				300,
				testIPv4A,
			),
			responseBody:   UnifiErrorResponse{Code: "INVALID", Message: "Invalid record"},
			responseStatus: http.StatusBadRequest,
			expectedErr:    true,
		},
		{
			name: "invalid SRV format",
			endpoint: endpoint.NewEndpointWithTTL(
				testSRVName,
				"SRV",
				300,
				"invalid srv format",
			),
			responseBody:   DNSRecord{},
			responseStatus: http.StatusOK,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Body != nil {
					capturedBody, _ = io.ReadAll(r.Body)
				}
				w.WriteHeader(tt.responseStatus)
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client := &httpClient{
				Config: &Config{
					Host:          server.URL,
					Site:          testSite,
					APIKey:        testKeyA,
					SkipTLSVerify: true,
				},
				Client: server.Client(),
				ClientURLs: &ClientURLs{
					Records: unifiRecordPathExternal,
				},
			}

			records, err := client.CreateEndpoint(context.Background(), tt.endpoint)

			if tt.expectedErr && err == nil {
				t.Error("CreateEndpoint() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("CreateEndpoint() unexpected error: %v", err)
			}

			if !tt.expectedErr && len(records) == 0 {
				t.Error("CreateEndpoint() returned no records")
			}

			if tt.validateReq != nil && len(capturedBody) > 0 {
				tt.validateReq(t, capturedBody)
			}
		})
	}
}

// TestDeleteEndpoint tests the DeleteEndpoint method.
func TestDeleteEndpoint(t *testing.T) {
	tests := []struct {
		name            string
		endpoint        *endpoint.Endpoint
		existingRecords []DNSRecord
		responseStatus  int
		expectedErr     bool
		expectedDeletes int
	}{
		{
			name: "delete single A record",
			endpoint: endpoint.NewEndpoint(
				testDomain,
				"A",
				testIPv4A,
			),
			existingRecords: []DNSRecord{
				{
					ID:         testDNSID1,
					Key:        testDomain,
					RecordType: recordTypeA,
					Value:      testIPv4A,
					TTL:        300,
					Enabled:    true,
				},
			},
			responseStatus:  http.StatusOK,
			expectedErr:     false,
			expectedDeletes: 1,
		},
		{
			name: "delete multiple A records with same name",
			endpoint: endpoint.NewEndpoint(
				testMultiName,
				"A",
			),
			existingRecords: []DNSRecord{
				{
					ID:         testDNSID1,
					Key:        testMultiName,
					RecordType: recordTypeA,
					Value:      testIPv4A,
					TTL:        300,
					Enabled:    true,
				},
				{
					ID:         testDNSID2,
					Key:        testMultiName,
					RecordType: recordTypeA,
					Value:      testIPv4B,
					TTL:        300,
					Enabled:    true,
				},
			},
			responseStatus:  http.StatusOK,
			expectedErr:     false,
			expectedDeletes: 2,
		},
		{
			name: "delete non-existent record",
			endpoint: endpoint.NewEndpoint(
				"nonexistent.example.com",
				"A",
				testIPv4A,
			),
			existingRecords: []DNSRecord{},
			responseStatus:  http.StatusOK,
			expectedErr:     false,
			expectedDeletes: 0,
		},
		{
			name: "delete CNAME record",
			endpoint: endpoint.NewEndpoint(
				testAlias,
				"CNAME",
				testTarget,
			),
			existingRecords: []DNSRecord{
				{
					ID:         testCNAMEID,
					Key:        testAlias,
					RecordType: recordTypeCNAME,
					Value:      testTarget,
					TTL:        300,
					Enabled:    true,
				},
			},
			responseStatus:  http.StatusOK,
			expectedErr:     false,
			expectedDeletes: 1,
		},
		{
			name: "HTTP 404 error on delete",
			endpoint: endpoint.NewEndpoint(
				testDomain,
				"A",
				testIPv4A,
			),
			existingRecords: []DNSRecord{
				{
					ID:         testDNSID1,
					Key:        testDomain,
					RecordType: recordTypeA,
					Value:      testIPv4A,
					TTL:        300,
					Enabled:    true,
				},
			},
			responseStatus:  http.StatusNotFound,
			expectedErr:     true,
			expectedDeletes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					// GetEndpoints call
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(tt.existingRecords)
				case http.MethodDelete:
					// Delete call
					deleteCount++
					w.WriteHeader(tt.responseStatus)
					if tt.responseStatus != http.StatusOK {
						_ = json.NewEncoder(w).Encode(UnifiErrorResponse{
							Code:    testErrCode,
							Message: "Delete failed",
						})
					}
				}
			}))
			defer server.Close()

			client := &httpClient{
				Config: &Config{
					Host:          server.URL,
					Site:          testSite,
					APIKey:        testKeyA,
					SkipTLSVerify: true,
				},
				Client: server.Client(),
				ClientURLs: &ClientURLs{
					Records: unifiRecordPathExternal,
				},
			}

			err := client.DeleteEndpoint(context.Background(), tt.endpoint)

			if tt.expectedErr && err == nil {
				t.Error("DeleteEndpoint() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("DeleteEndpoint() unexpected error: %v", err)
			}

			if deleteCount != tt.expectedDeletes {
				t.Errorf("DeleteEndpoint() made %d DELETE calls, want %d", deleteCount, tt.expectedDeletes)
			}
		})
	}
}

// TestSetHeaders tests header setting logic.
func TestSetHeaders(t *testing.T) {
	tests := []struct {
		name            string
		config          *Config
		csrf            string
		expectedHeaders map[string]string
	}{
		{
			name: "with API key",
			config: &Config{
				APIKey: testKeyB,
			},
			csrf: "",
			expectedHeaders: map[string]string{
				"X-Api-Key":       "test-api-key",
				headerAccept:      contentTypeJSON,
				headerContentType: contentTypeJSONUTF8,
			},
		},
		{
			name: "with CSRF token",
			config: &Config{
				APIKey: "",
			},
			csrf: testCSRF,
			expectedHeaders: map[string]string{
				"X-Csrf-Token":    testCSRF,
				headerAccept:      contentTypeJSON,
				headerContentType: contentTypeJSONUTF8,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &httpClient{
				Config: tt.config,
				csrf:   tt.csrf,
			}

			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", http.NoBody)
			client.setHeaders(req)

			for key, expectedValue := range tt.expectedHeaders {
				actualValue := req.Header.Get(key)
				if actualValue != expectedValue {
					t.Errorf("Header %s = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}

// TestFormatURL_ClientUsage tests FormatURL in real client scenarios.
func TestFormatURL_ClientUsage(t *testing.T) {
	tests := []struct {
		name      string
		urls      *ClientURLs
		host      string
		site      string
		recordID  string
		operation string
		expected  string
	}{
		{
			name: "internal controller - list records",
			urls: &ClientURLs{
				Records: unifiRecordPath,
			},
			host:      testHostPort,
			site:      testSite,
			recordID:  "",
			operation: testOpList,
			expected:  "https://192.168.1.1:8443/proxy/network/v2/api/site/default/static-dns/",
		},
		{
			name: "internal controller - get specific record",
			urls: &ClientURLs{
				Records: unifiRecordPath,
			},
			host:      testHostPort,
			site:      testSite,
			recordID:  "507f1f77bcf86cd799439011",
			operation: "get",
			expected:  "https://192.168.1.1:8443/proxy/network/v2/api/site/default/static-dns/507f1f77bcf86cd799439011",
		},
		{
			name: "external controller - list records",
			urls: &ClientURLs{
				Records: unifiRecordPathExternal,
			},
			host:      testHostExt,
			site:      "site-abc123",
			recordID:  "",
			operation: testOpList,
			expected:  "https://ui.com/v2/api/site/site-abc123/static-dns/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.recordID == "" {
				result = FormatURL(tt.urls.Records, tt.host, tt.site)
			} else {
				result = FormatURL(tt.urls.Records, tt.host, tt.site, tt.recordID)
			}

			if result != tt.expected {
				t.Errorf("FormatURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Helper function for tests.
func intPtr(i int) *int {
	return &i
}
