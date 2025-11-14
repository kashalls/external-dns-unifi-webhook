package unifi

import (
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
)

const (
	testSIPExampleDomain = "sip.example.com"
)

func TestRecordTransformer_PrepareDNSRecord(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       *endpoint.Endpoint
		target         string
		validateResult func(*testing.T, DNSRecord)
	}{
		{
			name: "A record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "test.example.com",
				RecordType: "A",
				RecordTTL:  300,
			},
			target: "192.168.1.1",
			validateResult: func(t *testing.T, record DNSRecord) {
				t.Helper()
				if record.Key != "test.example.com" {
					t.Errorf("Key = %q, want test.example.com", record.Key)
				}
				if record.RecordType != "A" {
					t.Errorf("RecordType = %q, want A", record.RecordType)
				}
				if record.TTL != 300 {
					t.Errorf("TTL = %d, want 300", record.TTL)
				}
				if record.Value != "192.168.1.1" {
					t.Errorf("Value = %q, want 192.168.1.1", record.Value)
				}
				if !record.Enabled {
					t.Error("Enabled = false, want true")
				}
			},
		},
		{
			name: "CNAME record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "alias.example.com",
				RecordType: "CNAME",
				RecordTTL:  600,
			},
			target: "target.example.com",
			validateResult: func(t *testing.T, record DNSRecord) {
				t.Helper()
				if record.Key != "alias.example.com" {
					t.Errorf("Key = %q, want alias.example.com", record.Key)
				}
				if record.RecordType != "CNAME" {
					t.Errorf("RecordType = %q, want CNAME", record.RecordType)
				}
				if record.Value != "target.example.com" {
					t.Errorf("Value = %q, want target.example.com", record.Value)
				}
			},
		},
		{
			name: "TXT record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "txt.example.com",
				RecordType: "TXT",
				RecordTTL:  3600,
			},
			target: "v=spf1 include:_spf.example.com ~all",
			validateResult: func(t *testing.T, record DNSRecord) {
				t.Helper()
				if record.RecordType != "TXT" {
					t.Errorf("RecordType = %q, want TXT", record.RecordType)
				}
				if record.Value != "v=spf1 include:_spf.example.com ~all" {
					t.Errorf("Value = %q, want v=spf1 include:_spf.example.com ~all", record.Value)
				}
			},
		},
		{
			name: "AAAA record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "ipv6.example.com",
				RecordType: "AAAA",
				RecordTTL:  300,
			},
			target: "2001:0db8::1",
			validateResult: func(t *testing.T, record DNSRecord) {
				t.Helper()
				if record.RecordType != "AAAA" {
					t.Errorf("RecordType = %q, want AAAA", record.RecordType)
				}
				if record.Value != "2001:0db8::1" {
					t.Errorf("Value = %q, want 2001:0db8::1", record.Value)
				}
			},
		},
		{
			name: "zero TTL",
			endpoint: &endpoint.Endpoint{
				DNSName:    "test.example.com",
				RecordType: "A",
				RecordTTL:  0,
			},
			target: "1.2.3.4",
			validateResult: func(t *testing.T, record DNSRecord) {
				t.Helper()
				if record.TTL != 0 {
					t.Errorf("TTL = %d, want 0", record.TTL)
				}
			},
		},
	}

	transformer := NewRecordTransformer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := transformer.PrepareDNSRecord(tt.endpoint, tt.target)

			if tt.validateResult != nil {
				tt.validateResult(t, record)
			}
		})
	}
}

func TestRecordTransformer_ParseSRVTarget(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		expectedErr    bool
		validateResult func(*testing.T, *DNSRecord)
	}{
		{
			name:        "valid SRV format",
			target:      "10 60 5060 " + testSIPExampleDomain,
			expectedErr: false,
			validateResult: func(t *testing.T, record *DNSRecord) {
				t.Helper()
				if record.Priority == nil {
					t.Fatal("Priority is nil")
				}
				if *record.Priority != 10 {
					t.Errorf("Priority = %d, want 10", *record.Priority)
				}
				if record.Weight == nil {
					t.Fatal("Weight is nil")
				}
				if *record.Weight != 60 {
					t.Errorf("Weight = %d, want 60", *record.Weight)
				}
				if record.Port == nil {
					t.Fatal("Port is nil")
				}
				if *record.Port != 5060 {
					t.Errorf("Port = %d, want 5060", *record.Port)
				}
				if record.Value != testSIPExampleDomain {
					t.Errorf("Value = %q, want sip.example.com", record.Value)
				}
			},
		},
		{
			name:        "valid SRV format with zero values",
			target:      "0 0 0 target.example.com",
			expectedErr: false,
			validateResult: func(t *testing.T, record *DNSRecord) {
				t.Helper()
				if record.Priority == nil || *record.Priority != 0 {
					t.Errorf("Priority = %v, want 0", record.Priority)
				}
				if record.Weight == nil || *record.Weight != 0 {
					t.Errorf("Weight = %v, want 0", record.Weight)
				}
				if record.Port == nil || *record.Port != 0 {
					t.Errorf("Port = %v, want 0", record.Port)
				}
			},
		},
		{
			name:        "valid SRV format with high values",
			target:      "65535 65535 65535 high.example.com",
			expectedErr: false,
			validateResult: func(t *testing.T, record *DNSRecord) {
				t.Helper()
				if record.Priority == nil || *record.Priority != 65535 {
					t.Errorf("Priority = %v, want 65535", record.Priority)
				}
				if record.Weight == nil || *record.Weight != 65535 {
					t.Errorf("Weight = %v, want 65535", record.Weight)
				}
				if record.Port == nil || *record.Port != 65535 {
					t.Errorf("Port = %v, want 65535", record.Port)
				}
			},
		},
		{
			name:        "invalid format - missing fields",
			target:      "10 60 sip.example.com",
			expectedErr: true,
		},
		{
			name:        "invalid format - non-numeric priority",
			target:      "abc 60 5060 sip.example.com",
			expectedErr: true,
		},
		{
			name:        "invalid format - non-numeric weight",
			target:      "10 abc 5060 sip.example.com",
			expectedErr: true,
		},
		{
			name:        "invalid format - non-numeric port",
			target:      "10 60 abc sip.example.com",
			expectedErr: true,
		},
		{
			name:        "invalid format - empty string",
			target:      "",
			expectedErr: true,
		},
		{
			name:        "invalid format - only numbers",
			target:      "10 60 5060",
			expectedErr: true,
		},
		{
			name:        "invalid format - extra fields",
			target:      "10 60 5060 " + testSIPExampleDomain + " extra",
			expectedErr: false, // Sscanf will parse first 4 fields successfully
			validateResult: func(t *testing.T, record *DNSRecord) {
				t.Helper()
				if record.Value != testSIPExampleDomain {
					t.Errorf("Value = %q, want sip.example.com (extra field should be ignored)", record.Value)
				}
			},
		},
	}

	transformer := NewRecordTransformer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := &DNSRecord{
				Key:        "_service._tcp.example.com",
				RecordType: "SRV",
			}

			err := transformer.ParseSRVTarget(record, tt.target)

			if tt.expectedErr && err == nil {
				t.Error("ParseSRVTarget() expected error, got nil")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("ParseSRVTarget() unexpected error: %v", err)
			}

			if !tt.expectedErr && tt.validateResult != nil {
				tt.validateResult(t, record)
			}
		})
	}
}

func TestRecordTransformer_FormatSRVValue(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		weight   int
		port     int
		target   string
		expected string
	}{
		{
			name:     "standard SRV values",
			priority: 10,
			weight:   60,
			port:     5060,
			target:   testSIPExampleDomain,
			expected: "10 60 5060 " + testSIPExampleDomain,
		},
		{
			name:     "zero values",
			priority: 0,
			weight:   0,
			port:     0,
			target:   "target.example.com",
			expected: "0 0 0 target.example.com",
		},
		{
			name:     "high priority",
			priority: 65535,
			weight:   100,
			port:     443,
			target:   "high.example.com",
			expected: "65535 100 443 high.example.com",
		},
		{
			name:     "HTTP service",
			priority: 1,
			weight:   10,
			port:     80,
			target:   "web.example.com",
			expected: "1 10 80 web.example.com",
		},
		{
			name:     "HTTPS service",
			priority: 5,
			weight:   50,
			port:     443,
			target:   "secure.example.com",
			expected: "5 50 443 secure.example.com",
		},
		{
			name:     "target with subdomain",
			priority: 10,
			weight:   20,
			port:     8080,
			target:   "service.subdomain.example.com",
			expected: "10 20 8080 service.subdomain.example.com",
		},
	}

	transformer := NewRecordTransformer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformer.FormatSRVValue(tt.priority, tt.weight, tt.port, tt.target)

			if result != tt.expected {
				t.Errorf("FormatSRVValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRecordTransformer_SRVRoundtrip(t *testing.T) {
	// Test that formatting and parsing are inverse operations
	tests := []struct {
		name     string
		priority int
		weight   int
		port     int
		target   string
	}{
		{
			name:     "standard values",
			priority: 10,
			weight:   60,
			port:     5060,
			target:   testSIPExampleDomain,
		},
		{
			name:     "zero values",
			priority: 0,
			weight:   0,
			port:     0,
			target:   "target.example.com",
		},
		{
			name:     "maximum values",
			priority: 65535,
			weight:   65535,
			port:     65535,
			target:   "max.example.com",
		},
	}

	transformer := NewRecordTransformer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Format values to string
			formatted := transformer.FormatSRVValue(tt.priority, tt.weight, tt.port, tt.target)

			// Parse back to record
			record := &DNSRecord{}
			err := transformer.ParseSRVTarget(record, formatted)
			if err != nil {
				t.Fatalf("ParseSRVTarget() error = %v", err)
			}

			// Verify values match
			if record.Priority == nil || *record.Priority != tt.priority {
				t.Errorf("Priority = %v, want %d", record.Priority, tt.priority)
			}
			if record.Weight == nil || *record.Weight != tt.weight {
				t.Errorf("Weight = %v, want %d", record.Weight, tt.weight)
			}
			if record.Port == nil || *record.Port != tt.port {
				t.Errorf("Port = %v, want %d", record.Port, tt.port)
			}
			if record.Value != tt.target {
				t.Errorf("Value = %q, want %q", record.Value, tt.target)
			}
		})
	}
}
