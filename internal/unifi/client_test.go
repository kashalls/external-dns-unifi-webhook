package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/home-operations/external-dns-unifi-webhook/internal/metrics"
	"sigs.k8s.io/external-dns/endpoint"
)

func init() {
	// Force lazy initialization of the metrics singleton so handler paths can
	// rely on metrics.Get() returning a non-nil instance.
	_ = metrics.Get()
}

const (
	testDomain = "test.example.com"
)

// newTestClient builds an httpClient pointed at the given test server, with
// the site UUID already resolved. Real client construction goes through
// resolveSite; tests skip that by populating siteID directly.
func newTestClient(srv *httptest.Server) *httpClient {
	return testClient(srv, &Config{
		Host:          srv.URL,
		Site:          testSite,
		APIKey:        testKeyA,
		SkipTLSVerify: true,
	})
}

// TestCloudConnectorPath verifies that with a console ID set, every request is
// routed through the api.ui.com connector prefix while reusing the same
// Integration API endpoints — the whole point of the cloud support is that
// only the base URL changes.
func TestCloudConnectorPath(t *testing.T) {
	wantPrefix := "/v1/connector/consoles/" + testConsoleID + "/proxy/network/integration/v1"

	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(pageOf(envA(testDNSID1, testDomain, testIPv4A, 300)))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := testClient(srv, &Config{
		Host:          srv.URL,
		Site:          testSite,
		APIKey:        testKeyA,
		ConsoleID:     testConsoleID,
		SkipTLSVerify: true,
	})

	if _, err := c.GetEndpoints(context.Background()); err != nil {
		t.Fatalf("GetEndpoints: %v", err)
	}
	if err := c.DeleteRecord(context.Background(), testDNSID1); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	if len(gotPaths) == 0 {
		t.Fatal("server received no requests")
	}
	for _, p := range gotPaths {
		if !strings.HasPrefix(p, wantPrefix) {
			t.Errorf("request path %q does not start with cloud connector prefix %q", p, wantPrefix)
		}
	}
	// Sanity-check the suffix is the same Integration API surface as local.
	if want := wantPrefix + "/sites/" + testSiteUUID + "/dns/policies/" + testDNSID1; gotPaths[len(gotPaths)-1] != want {
		t.Errorf("delete path = %q, want %q", gotPaths[len(gotPaths)-1], want)
	}
}

// envelope helpers keep test data terse and consistent.
func envA(id, domain, ip string, ttl int) dnsPolicyEnvelope {
	return dnsPolicyEnvelope{
		ID: id, Type: policyTypeA, Enabled: true,
		Domain: domain, IPv4Address: ip, TTLSeconds: &ttl,
	}
}

func envAAAA(id, domain, ip string, ttl int) dnsPolicyEnvelope {
	return dnsPolicyEnvelope{
		ID: id, Type: policyTypeAAAA, Enabled: true,
		Domain: domain, IPv6Address: ip, TTLSeconds: &ttl,
	}
}

func envCNAME(id, domain, target string, ttl int) dnsPolicyEnvelope {
	return dnsPolicyEnvelope{
		ID: id, Type: policyTypeCNAME, Enabled: true,
		Domain: domain, TargetDomain: target, TTLSeconds: &ttl,
	}
}

func envTXT(id, domain, text string) dnsPolicyEnvelope {
	return dnsPolicyEnvelope{
		ID: id, Type: policyTypeTXT, Enabled: true,
		Domain: domain, Text: text,
	}
}

func envSRV(id, service, protocol, domain, server string, priority, weight, port int) dnsPolicyEnvelope {
	return dnsPolicyEnvelope{
		ID: id, Type: policyTypeSRV, Enabled: true,
		Domain: domain, Service: service, Protocol: protocol,
		ServerDomain: server,
		Priority:     &priority,
		Weight:       &weight,
		Port:         &port,
	}
}

func pageOf(envs ...dnsPolicyEnvelope) dnsPolicyPage {
	return dnsPolicyPage{
		Offset:     0,
		Limit:      pageLimit,
		Count:      len(envs),
		TotalCount: len(envs),
		Data:       envs,
	}
}

// TestGetEndpoints exercises the paginated list path against a faked server.
func TestGetEndpoints(t *testing.T) {
	srvPriority, srvWeight, srvPort := 10, 20, 8080
	srvEnv := envSRV("srv1", "_service", "_tcp", "example.com", testTarget,
		srvPriority, srvWeight, srvPort)

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
			responseBody: pageOf(
				envA(testDNSID1, testDomain, testIPv4A, 300),
				envA(testDNSID2, "test2.example.com", testIPv4B, 600),
			),
			responseStatus: http.StatusOK,
			expectedLen:    2,
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
			name:           "SRV record reassembled into external-dns shape",
			responseBody:   pageOf(srvEnv),
			responseStatus: http.StatusOK,
			expectedLen:    1,
			validateResult: func(t *testing.T, records []DNSRecord) {
				t.Helper()
				if records[0].Key != testSRVName {
					t.Errorf("SRV Key = %q, want %q", records[0].Key, testSRVName)
				}
				expected := "10 20 8080 target.example.com"
				if records[0].Value != expected {
					t.Errorf("SRV Value = %q, want %q", records[0].Value, expected)
				}
			},
		},
		{
			name: "FORWARD_DOMAIN is filtered out",
			responseBody: pageOf(
				envA(testDNSID1, testDomain, testIPv4A, 300),
				dnsPolicyEnvelope{
					ID: "fwd1", Type: policyTypeForward, Enabled: true,
					Domain: "internal.example.com", IPAddress: "10.0.0.53",
				},
			),
			responseStatus: http.StatusOK,
			expectedLen:    1,
		},
		{
			name:           "empty response",
			responseBody:   pageOf(),
			responseStatus: http.StatusOK,
			expectedLen:    0,
		},
		{
			name:           "invalid JSON response",
			responseBody:   `{"invalid": json}`,
			responseStatus: http.StatusOK,
			expectedLen:    0,
			expectedErr:    true,
		},
		{
			name: "HTTP 500 error",
			responseBody: apiErrorResponse{
				StatusCode: 500, StatusName: "INTERNAL_SERVER_ERROR",
				Code: testErrCode, Message: testMsgInternalServer,
			},
			responseStatus: http.StatusInternalServerError,
			expectedLen:    0,
			expectedErr:    true,
		},
		{
			name: "mixed record types",
			responseBody: pageOf(
				envA("a1", "a.example.com", "1.2.3.4", 300),
				envCNAME(testCNAMEID, "cname.example.com", testTarget, 300),
				envTXT("txt1", "txt.example.com", "v=spf1 include:example.com ~all"),
			),
			responseStatus: http.StatusOK,
			expectedLen:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.responseStatus)
				if strBody, ok := tt.responseBody.(string); ok {
					_, _ = w.Write([]byte(strBody))

					return
				}
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client := newTestClient(server)

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

// TestGetEndpoints_Pagination forces the client to walk two pages and verifies
// it sees every record exactly once.
func TestGetEndpoints_Pagination(t *testing.T) {
	const total = 350
	first := make([]dnsPolicyEnvelope, pageLimit)
	for i := range first {
		s := strconv.Itoa(i)
		first[i] = envA("a"+s, "host"+s+".example.com", "192.0.2.1", 300)
	}
	second := make([]dnsPolicyEnvelope, total-pageLimit)
	for i := range second {
		s := strconv.Itoa(i)
		second[i] = envA("b"+s, "more"+s+".example.com", "192.0.2.2", 300)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		var data []dnsPolicyEnvelope
		off := 0
		switch offset {
		case "0", "":
			data = first
		case strconv.Itoa(pageLimit):
			data = second
			off = pageLimit
		default:
			t.Errorf("unexpected offset %q", offset)
		}
		_ = json.NewEncoder(w).Encode(dnsPolicyPage{
			Offset:     off,
			Limit:      pageLimit,
			Count:      len(data),
			TotalCount: total,
			Data:       data,
		})
	}))
	defer srv.Close()

	records, err := newTestClient(srv).GetEndpoints(context.Background())
	if err != nil {
		t.Fatalf("GetEndpoints: %v", err)
	}
	if got := len(records); got != total {
		t.Errorf("expected %d records across pages, got %d", total, got)
	}
}

// validateARequest checks an A-record POST body has the expected envelope shape.
func validateARequest(t *testing.T, body []byte) {
	t.Helper()
	var env dnsPolicyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Type != policyTypeA {
		t.Errorf("Type = %q, want %q", env.Type, policyTypeA)
	}
	if env.Domain != testDomain {
		t.Errorf("Domain = %q, want %q", env.Domain, testDomain)
	}
	if env.IPv4Address != testIPv4A {
		t.Errorf("IPv4Address = %q, want %q", env.IPv4Address, testIPv4A)
	}
	if env.TTLSeconds == nil || *env.TTLSeconds != 300 {
		t.Errorf("TTLSeconds = %v, want 300", env.TTLSeconds)
	}
}

// validateSRVRequest checks an SRV-record POST body splits name and value.
func validateSRVRequest(t *testing.T, body []byte) {
	t.Helper()
	var env dnsPolicyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Service != "_service" {
		t.Errorf("Service = %q, want _service", env.Service)
	}
	if env.Protocol != "_tcp" {
		t.Errorf("Protocol = %q, want _tcp", env.Protocol)
	}
	if env.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", env.Domain)
	}
	if env.Priority == nil || *env.Priority != 10 {
		t.Errorf("Priority = %v, want 10", env.Priority)
	}
	if env.Weight == nil || *env.Weight != 20 {
		t.Errorf("Weight = %v, want 20", env.Weight)
	}
	if env.Port == nil || *env.Port != 8080 {
		t.Errorf("Port = %v, want 8080", env.Port)
	}
	if env.ServerDomain != testTarget {
		t.Errorf("ServerDomain = %q, want %q", env.ServerDomain, testTarget)
	}
}

// validateTTLClamped checks A-record TTL gets clamped at the spec maximum.
func validateTTLClamped(t *testing.T, body []byte) {
	t.Helper()
	var env dnsPolicyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.TTLSeconds == nil || *env.TTLSeconds != ttlMaxA {
		t.Errorf("TTLSeconds = %v, want %d", env.TTLSeconds, ttlMaxA)
	}
}

// TestCreateEndpoint tests the per-type envelope construction on the create
// path, plus error handling for invalid SRV/MX targets.
func TestCreateEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       *endpoint.Endpoint
		responseEnv    dnsPolicyEnvelope
		responseStatus int
		expectedErr    bool
		validateReq    func(*testing.T, []byte)
	}{
		{
			name:           "create A record",
			endpoint:       endpoint.NewEndpointWithTTL(testDomain, "A", 300, testIPv4A),
			responseEnv:    envA("new-record-1", testDomain, testIPv4A, 300),
			responseStatus: http.StatusOK,
			validateReq:    validateARequest,
		},
		{
			name:           "create CNAME record",
			endpoint:       endpoint.NewEndpointWithTTL(testAlias, "CNAME", 600, testTarget),
			responseEnv:    envCNAME("new-cname-1", testAlias, testTarget, 600),
			responseStatus: http.StatusOK,
		},
		{
			name:           "create SRV record splits name and value",
			endpoint:       endpoint.NewEndpointWithTTL(testSRVName, "SRV", 300, "10 20 8080 target.example.com"),
			responseEnv:    envSRV("new-srv-1", "_service", "_tcp", "example.com", testTarget, 10, 20, 8080),
			responseStatus: http.StatusOK,
			validateReq:    validateSRVRequest,
		},
		{
			name:           "TTL clamped to A-record maximum",
			endpoint:       endpoint.NewEndpointWithTTL(testDomain, "A", 999999, testIPv4A),
			responseEnv:    envA("a-clamped", testDomain, testIPv4A, ttlMaxA),
			responseStatus: http.StatusOK,
			validateReq:    validateTTLClamped,
		},
		{
			name: "create multiple CNAME targets - uses only first",
			endpoint: endpoint.NewEndpointWithTTL(
				testMultiName, "CNAME", 300,
				"target1.example.com", "target2.example.com",
			),
			responseEnv:    envCNAME("new-record", testMultiName, "target1.example.com", 300),
			responseStatus: http.StatusOK,
		},
		{
			name:           "HTTP 400 error",
			endpoint:       endpoint.NewEndpointWithTTL(testDomain, "A", 300, testIPv4A),
			responseEnv:    dnsPolicyEnvelope{},
			responseStatus: http.StatusBadRequest,
			expectedErr:    true,
		},
		{
			name:           "invalid SRV format",
			endpoint:       endpoint.NewEndpointWithTTL(testSRVName, "SRV", 300, "invalid srv format"),
			responseEnv:    dnsPolicyEnvelope{},
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
				if tt.responseStatus >= 400 {
					_ = json.NewEncoder(w).Encode(apiErrorResponse{
						StatusCode: tt.responseStatus, StatusName: "BAD_REQUEST",
						Code: "validation.failed", Message: "Invalid record",
					})

					return
				}
				_ = json.NewEncoder(w).Encode(tt.responseEnv)
			}))
			defer server.Close()

			records, err := newTestClient(server).CreateEndpoint(context.Background(), tt.endpoint)

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

// TestDeleteRecord covers the single-record DELETE path, including the
// idempotent treatment of 404 responses.
func TestDeleteRecord(t *testing.T) {
	tests := []struct {
		name        string
		recordID    string
		respStatus  int
		expectedErr bool
	}{
		{
			name:       "delete success",
			recordID:   testDNSID1,
			respStatus: http.StatusOK,
		},
		{
			name:       "404 is treated as success (already deleted)",
			recordID:   "missing",
			respStatus: http.StatusNotFound,
		},
		{
			name:        "500 surfaces as error",
			recordID:    testDNSID1,
			respStatus:  http.StatusInternalServerError,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotMethod, gotPath string
				deleteCount        int
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				deleteCount++
				w.WriteHeader(tt.respStatus)
				if tt.respStatus >= 400 {
					_ = json.NewEncoder(w).Encode(apiErrorResponse{
						StatusCode: tt.respStatus, StatusName: "ERROR",
						Code: testErrCode, Message: "Delete failed",
					})
				}
			}))
			defer server.Close()

			err := newTestClient(server).DeleteRecord(context.Background(), tt.recordID)

			if tt.expectedErr && err == nil {
				t.Error("DeleteRecord() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("DeleteRecord() unexpected error: %v", err)
			}
			if deleteCount != 1 {
				t.Errorf("DeleteRecord() made %d DELETE calls, want 1", deleteCount)
			}
			if gotMethod != http.MethodDelete {
				t.Errorf("method = %q, want DELETE", gotMethod)
			}
			if !strings.HasSuffix(gotPath, "/dns/policies/"+tt.recordID) {
				t.Errorf("path = %q, expected suffix /dns/policies/%s", gotPath, tt.recordID)
			}
		})
	}
}

// TestResolveSite covers the startup site-UUID lookup, including the
// internalReference / UUID heuristic and the empty/multi-match error paths.
func TestResolveSite(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		data       []siteEntry
		wantField  string // "internalReference" or "id"
		wantFilter string // exact filter expression, when escaping matters
		wantID     string
		wantErr    bool
	}{
		{
			name:      "by name resolves to UUID",
			ref:       "default",
			data:      []siteEntry{{ID: testSiteUUID, InternalReference: "default", Name: "Default"}},
			wantField: "internalReference",
			wantID:    testSiteUUID,
		},
		{
			name:       "single quote in site name is doubled",
			ref:        "o'brien",
			data:       []siteEntry{{ID: testSiteUUID, InternalReference: "o'brien", Name: "O'Brien"}},
			wantFilter: "internalReference.eq('o''brien')",
			wantID:     testSiteUUID,
		},
		{
			name:      "UUID input filters by id",
			ref:       testSiteUUID,
			data:      []siteEntry{{ID: testSiteUUID, InternalReference: "default", Name: "Default"}},
			wantField: "id",
			wantID:    testSiteUUID,
		},
		{
			name:    "no match errors",
			ref:     "missing",
			data:    []siteEntry{},
			wantErr: true,
		},
		{
			name: "multi match errors",
			ref:  "default",
			data: []siteEntry{
				{ID: testSiteUUID, InternalReference: "default", Name: "A"},
				{ID: "22222222-2222-2222-2222-222222222222", InternalReference: "default", Name: "B"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedFilter string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedFilter = r.URL.Query().Get("filter")
				_ = json.NewEncoder(w).Encode(sitePage{
					Offset: 0, Limit: 25,
					Count: len(tt.data), TotalCount: len(tt.data),
					Data: tt.data,
				})
			}))
			defer srv.Close()

			c := &httpClient{
				cfg:   &Config{Host: srv.URL, APIKey: testKeyA, SkipTLSVerify: true},
				httpc: srv.Client(),
			}

			id, err := c.resolveSite(context.Background(), tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}
			if err != nil {
				t.Fatalf("resolveSite: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("siteID = %q, want %q", id, tt.wantID)
			}
			if tt.wantField != "" && !strings.HasPrefix(capturedFilter, tt.wantField+".eq(") {
				t.Errorf("filter = %q, expected prefix %s.eq(", capturedFilter, tt.wantField)
			}
			if tt.wantFilter != "" && capturedFilter != tt.wantFilter {
				t.Errorf("filter = %q, want %q", capturedFilter, tt.wantFilter)
			}
		})
	}
}

// TestSetHeaders verifies the API-key auth header and standard JSON headers.
func TestSetHeaders(t *testing.T) {
	client := &httpClient{cfg: &Config{APIKey: testKeyB}}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", http.NoBody)
	client.setHeaders(req)

	expected := map[string]string{
		"X-API-KEY":       "test-api-key",
		headerAccept:      contentTypeJSON,
		headerContentType: contentTypeJSONUTF8,
	}
	for key, want := range expected {
		if got := req.Header.Get(key); got != want {
			t.Errorf("Header %s = %q, want %q", key, got, want)
		}
	}
}

// TestDecodeJSON_RejectsOversizedBody proves decodeJSON refuses to buffer a
// response larger than maxResponseBytes instead of letting io.ReadAll grow
// unbounded. These two tests are intentionally serial (no t.Parallel) because
// they mutate the package-level cap; Go resumes parallel tests only after the
// serial group finishes, so the deferred restore closes the window first.
func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	defer func(orig int64) { maxResponseBytes = orig }(maxResponseBytes)
	maxResponseBytes = 16

	// Valid JSON, but well past the 16-byte cap. Decoding into a generic sink
	// means the only thing that can fail here is the size guard — the old
	// unbounded io.ReadAll would have buffered it all and returned nil.
	oversized := `{"data":[` + strings.Repeat(`"x",`, 100) + `"x"]}`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(oversized))}

	var dest any
	n, err := decodeJSON(resp, "DNS policies", &dest)
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}

	var dataErr *DataError
	if !errors.As(err, &dataErr) {
		t.Fatalf("expected *DataError, got %T: %v", err, err)
	}
	if !strings.Contains(dataErr.Error(), "limit") {
		t.Errorf("error %q should mention the byte limit", dataErr.Error())
	}
	// Rejection must happen before unmarshalling, so dest stays nil.
	if dest != nil {
		t.Errorf("oversized body must not be decoded: got %v", dest)
	}
	// The over-limit read is reported (cap+1), never silently zeroed.
	if int64(n) <= maxResponseBytes {
		t.Errorf("byte count = %d, want > cap %d", n, maxResponseBytes)
	}
}

// TestDecodeJSON_AcceptsBodyAtLimit guards the boundary: a body sitting exactly
// at maxResponseBytes still decodes (the +1 LimitReader headroom is what lets
// it through), so a tight-but-legitimate payload is never rejected.
func TestDecodeJSON_AcceptsBodyAtLimit(t *testing.T) {
	defer func(orig int64) { maxResponseBytes = orig }(maxResponseBytes)

	body := `{}`
	maxResponseBytes = int64(len(body)) // exactly at the cap

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	var dest dnsPolicyPage
	if _, err := decodeJSON(resp, "DNS policies", &dest); err != nil {
		t.Fatalf("body exactly at the limit should decode, got: %v", err)
	}
}
