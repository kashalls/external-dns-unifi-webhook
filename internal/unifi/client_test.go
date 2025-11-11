package unifi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kashalls/external-dns-unifi-webhook/cmd/webhook/init/log"
	"github.com/kashalls/external-dns-unifi-webhook/pkg/metrics"
	"sigs.k8s.io/external-dns/endpoint"
)

func init() {
	log.Init()
}

// mockHTTPTransport is a mock implementation of HTTPTransport for testing
type mockHTTPTransport struct {
	doRequestFunc func(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
	loginFunc     func(ctx context.Context) error
}

func (m *mockHTTPTransport) DoRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if m.doRequestFunc != nil {
		return m.doRequestFunc(ctx, method, path, body)
	}
	return nil, nil
}

func (m *mockHTTPTransport) Login(ctx context.Context) error {
	if m.loginFunc != nil {
		return m.loginFunc(ctx)
	}
	return nil
}

func (m *mockHTTPTransport) SetHeaders(_ *http.Request) {}

func (m *mockHTTPTransport) GetClientURLs() *ClientURLs {
	return &ClientURLs{
		Login:   "%s/api/auth/login",
		Records: "%s/v2/api/site/%s/static-dns/%s",
	}
}

func (m *mockHTTPTransport) GetConfig() *Config {
	return &Config{
		Host: "https://unifi.example.com",
		Site: "default",
	}
}

func TestUnifiAPIClient_GetEndpoints_Success(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, path string, _ io.Reader) (*http.Response, error) {
			if method != http.MethodGet {
				t.Errorf("Expected GET request, got %s", method)
			}

			body := `[
				{"_id": "1", "key": "test.example.com", "record_type": "A", "value": "1.2.3.4", "ttl": 300, "enabled": true},
				{"_id": "2", "key": "_service._tcp.example.com", "record_type": "SRV", "value": "target.example.com", "ttl": 300, "enabled": true, "port": 8080, "priority": 10, "weight": 20}
			]`

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	records, err := client.GetEndpoints(context.Background())
	if err != nil {
		t.Fatalf("GetEndpoints() error = %v", err)
	}

	if len(records) != 2 {
		t.Errorf("GetEndpoints() returned %d records, want 2", len(records))
	}

	// Check A record
	if records[0].RecordType != "A" {
		t.Errorf("First record type = %s, want A", records[0].RecordType)
	}
	if records[0].Value != "1.2.3.4" {
		t.Errorf("First record value = %s, want 1.2.3.4", records[0].Value)
	}

	// Check SRV record transformation
	if records[1].RecordType != "SRV" {
		t.Errorf("Second record type = %s, want SRV", records[1].RecordType)
	}
	if records[1].Value != "10 20 8080 target.example.com" {
		t.Errorf("Second record value = %s, want '10 20 8080 target.example.com'", records[1].Value)
	}
}

func TestUnifiAPIClient_GetEndpoints_EmptyResponse(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, _, _ string, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("[]")),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	records, err := client.GetEndpoints(context.Background())
	if err != nil {
		t.Fatalf("GetEndpoints() error = %v", err)
	}

	if len(records) != 0 {
		t.Errorf("GetEndpoints() returned %d records, want 0", len(records))
	}
}

func TestUnifiAPIClient_GetEndpoints_NetworkError(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, _, path string, _ io.Reader) (*http.Response, error) {
			return nil, NewNetworkError("GET", path, io.EOF)
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	_, err := client.GetEndpoints(context.Background())
	if err == nil {
		t.Fatal("GetEndpoints() expected error, got nil")
	}
}

func TestUnifiAPIClient_GetEndpoints_InvalidJSON(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, _, _ string, _ io.Reader) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{invalid json}")),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	_, err := client.GetEndpoints(context.Background())
	if err == nil {
		t.Fatal("GetEndpoints() expected error for invalid JSON, got nil")
	}
}

func TestUnifiAPIClient_CreateEndpoint_ARecord(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, body io.Reader) (*http.Response, error) {
			if method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", method)
			}

			responseBody := `{"_id": "new-id", "key": "test.example.com", "record_type": "A", "value": "1.2.3.4", "ttl": 300, "enabled": true}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "test.example.com",
		RecordType: "A",
		Targets:    []string{"1.2.3.4"},
		RecordTTL:  300,
	}

	records, err := client.CreateEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}

	if len(records) != 1 {
		t.Errorf("CreateEndpoint() returned %d records, want 1", len(records))
	}

	if records[0].RecordType != "A" {
		t.Errorf("Record type = %s, want A", records[0].RecordType)
	}
}

func TestUnifiAPIClient_CreateEndpoint_CNAMERecord(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, _ io.Reader) (*http.Response, error) {
			if method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", method)
			}

			responseBody := `{"_id": "new-id", "key": "alias.example.com", "record_type": "CNAME", "value": "target.example.com", "ttl": 300, "enabled": true}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "alias.example.com",
		RecordType: "CNAME",
		Targets:    []string{"target.example.com"},
		RecordTTL:  300,
	}

	records, err := client.CreateEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}

	if len(records) != 1 {
		t.Errorf("CreateEndpoint() returned %d records, want 1", len(records))
	}
}

func TestUnifiAPIClient_CreateEndpoint_SRVRecord(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, _ io.Reader) (*http.Response, error) {
			if method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", method)
			}

			responseBody := `{"_id": "new-id", "key": "_service._tcp.example.com", "record_type": "SRV", "value": "target.example.com", "ttl": 300, "enabled": true, "port": 8080, "priority": 10, "weight": 20}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "_service._tcp.example.com",
		RecordType: "SRV",
		Targets:    []string{"10 20 8080 target.example.com"},
		RecordTTL:  300,
	}

	records, err := client.CreateEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}

	if len(records) != 1 {
		t.Errorf("CreateEndpoint() returned %d records, want 1", len(records))
	}
}

func TestUnifiAPIClient_CreateEndpoint_MultipleTargets(t *testing.T) {
	callCount := 0
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, _ io.Reader) (*http.Response, error) {
			callCount++
			responseBody := `{"_id": "new-id", "key": "multi.example.com", "record_type": "A", "value": "1.2.3.4", "ttl": 300, "enabled": true}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "multi.example.com",
		RecordType: "A",
		Targets:    []string{"1.2.3.4", "5.6.7.8"},
		RecordTTL:  300,
	}

	records, err := client.CreateEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}

	if len(records) != 2 {
		t.Errorf("CreateEndpoint() returned %d records, want 2", len(records))
	}

	if callCount != 2 {
		t.Errorf("CreateEndpoint() made %d API calls, want 2", callCount)
	}
}

func TestUnifiAPIClient_CreateEndpoint_InvalidSRV(t *testing.T) {
	mockTransport := &mockHTTPTransport{}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "_service._tcp.example.com",
		RecordType: "SRV",
		Targets:    []string{"invalid-srv-format"},
		RecordTTL:  300,
	}

	_, err := client.CreateEndpoint(context.Background(), ep)
	if err == nil {
		t.Fatal("CreateEndpoint() expected error for invalid SRV format, got nil")
	}
}

func TestUnifiAPIClient_DeleteEndpoint_SingleRecord(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, path string, _ io.Reader) (*http.Response, error) {
			if method == http.MethodGet {
				body := `[{"_id": "1", "key": "test.example.com", "record_type": "A", "value": "1.2.3.4", "ttl": 300, "enabled": true}]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}
			if method == http.MethodDelete {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{}")),
				}, nil
			}
			return nil, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "test.example.com",
		RecordType: "A",
	}

	err := client.DeleteEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("DeleteEndpoint() error = %v", err)
	}
}

func TestUnifiAPIClient_DeleteEndpoint_MultipleRecords(t *testing.T) {
	deleteCount := 0
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, _ io.Reader) (*http.Response, error) {
			if method == http.MethodGet {
				body := `[
					{"_id": "1", "key": "test.example.com", "record_type": "A", "value": "1.2.3.4", "ttl": 300, "enabled": true},
					{"_id": "2", "key": "test.example.com", "record_type": "A", "value": "5.6.7.8", "ttl": 300, "enabled": true}
				]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}
			if method == http.MethodDelete {
				deleteCount++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{}")),
				}, nil
			}
			return nil, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "test.example.com",
		RecordType: "A",
	}

	err := client.DeleteEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("DeleteEndpoint() error = %v", err)
	}

	if deleteCount != 2 {
		t.Errorf("DeleteEndpoint() made %d DELETE calls, want 2", deleteCount)
	}
}

func TestUnifiAPIClient_DeleteEndpoint_NoMatchingRecords(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, _ string, _ io.Reader) (*http.Response, error) {
			if method == http.MethodGet {
				body := `[]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}
			return nil, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "nonexistent.example.com",
		RecordType: "A",
	}

	err := client.DeleteEndpoint(context.Background(), ep)
	if err != nil {
		t.Fatalf("DeleteEndpoint() error = %v (should succeed with no records)", err)
	}
}

func TestUnifiAPIClient_DeleteEndpoint_GetError(t *testing.T) {
	mockTransport := &mockHTTPTransport{
		doRequestFunc: func(_ context.Context, method, path string, _ io.Reader) (*http.Response, error) {
			if method == http.MethodGet {
				return nil, NewNetworkError("GET", path, io.EOF)
			}
			return nil, nil
		},
	}

	client := NewUnifiAPIClient(
		mockTransport,
		NewRecordTransformer(),
		NewMetricsAdapter(metrics.New("test")),
		NewLoggerAdapter(),
		&Config{Host: "https://unifi.example.com", Site: "default"},
		&ClientURLs{Records: "%s/v2/api/site/%s/static-dns/%s"},
	)

	ep := &endpoint.Endpoint{
		DNSName:    "test.example.com",
		RecordType: "A",
	}

	err := client.DeleteEndpoint(context.Background(), ep)
	if err == nil {
		t.Fatal("DeleteEndpoint() expected error when GET fails, got nil")
	}
}

func TestNewUnifiClient_Deprecated(t *testing.T) {
	config := &Config{
		Host:   "https://unifi.example.com",
		Site:   "default",
		APIKey: "test-key",
	}

	client, err := newUnifiClient(config)
	if err != nil {
		t.Fatalf("newUnifiClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("newUnifiClient() returned nil")
	}
}
