package unifi

import (
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
)

func TestToDNSRecord_PerType(t *testing.T) {
	tests := []struct {
		name      string
		env       dnsPolicyEnvelope
		wantOK    bool
		wantKey   string
		wantValue string
		wantType  string
		wantTTL   endpoint.TTL
	}{
		{
			name:      "A record",
			env:       envA("a1", "host.example.com", "192.0.2.1", 300),
			wantOK:    true,
			wantKey:   "host.example.com",
			wantValue: "192.0.2.1",
			wantType:  recordTypeA,
			wantTTL:   300,
		},
		{
			name: "AAAA record",
			env: dnsPolicyEnvelope{
				Type: policyTypeAAAA, Enabled: true,
				Domain: "v6.example.com", IPv6Address: "2001:db8::1",
				TTLSeconds: new(60),
			},
			wantOK:    true,
			wantKey:   "v6.example.com",
			wantValue: "2001:db8::1",
			wantType:  recordTypeAAAA,
			wantTTL:   60,
		},
		{
			name:      "CNAME record",
			env:       envCNAME("c1", "alias.example.com", "target.example.com", 120),
			wantOK:    true,
			wantKey:   "alias.example.com",
			wantValue: "target.example.com",
			wantType:  recordTypeCNAME,
			wantTTL:   120,
		},
		{
			name:      "TXT has no TTL field on the wire",
			env:       envTXT("t1", "txt.example.com", "v=spf1 ~all"),
			wantOK:    true,
			wantKey:   "txt.example.com",
			wantValue: "v=spf1 ~all",
			wantType:  recordTypeTXT,
		},
		{
			name: "MX renders as 'priority host'",
			env: dnsPolicyEnvelope{
				Type: policyTypeMX, Enabled: true,
				Domain: "example.com", MailServerDomain: "mail.example.com",
				Priority: new(10),
			},
			wantOK:    true,
			wantKey:   "example.com",
			wantValue: "10 mail.example.com",
			wantType:  recordTypeMX,
		},
		{
			name:      "SRV reassembled into _service._proto.domain and target",
			env:       envSRV("s1", "_ldap", "_tcp", "example.com", "ldap.example.com", 10, 20, 389),
			wantOK:    true,
			wantKey:   "_ldap._tcp.example.com",
			wantValue: "10 20 389 ldap.example.com",
			wantType:  recordTypeSRV,
		},
		{
			name: "FORWARD_DOMAIN is dropped",
			env: dnsPolicyEnvelope{
				Type: policyTypeForward, Enabled: true,
				Domain: "internal.example.com", IPAddress: "10.0.0.53",
			},
			wantOK: false,
		},
		{
			name:   "unknown type is dropped",
			env:    dnsPolicyEnvelope{Type: "MYSTERY", Domain: "x"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toDNSRecord(tt.env)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", got.Key, tt.wantKey)
			}
			if got.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", got.Value, tt.wantValue)
			}
			if got.RecordType != tt.wantType {
				t.Errorf("RecordType = %q, want %q", got.RecordType, tt.wantType)
			}
			if got.TTL != tt.wantTTL {
				t.Errorf("TTL = %v, want %v", got.TTL, tt.wantTTL)
			}
		})
	}
}

func TestFromDNSRecord_PerType(t *testing.T) {
	tests := []struct {
		name    string
		in      DNSRecord
		check   func(*testing.T, dnsPolicyEnvelope)
		wantErr bool
	}{
		{
			name: "A record",
			in:   DNSRecord{Key: "host.example.com", RecordType: recordTypeA, Value: "192.0.2.1", TTL: 300},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.Type != policyTypeA {
					t.Errorf("Type = %q", env.Type)
				}
				if env.IPv4Address != "192.0.2.1" {
					t.Errorf("IPv4Address = %q", env.IPv4Address)
				}
				if env.TTLSeconds == nil || *env.TTLSeconds != 300 {
					t.Errorf("TTLSeconds = %v", env.TTLSeconds)
				}
			},
		},
		{
			name: "A TTL clamped at 86400",
			in:   DNSRecord{Key: "x.example.com", RecordType: recordTypeA, Value: "192.0.2.1", TTL: 999999},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.TTLSeconds == nil || *env.TTLSeconds != ttlMaxA {
					t.Errorf("TTLSeconds = %v, want %d", env.TTLSeconds, ttlMaxA)
				}
			},
		},
		{
			name: "CNAME TTL clamped at 604800",
			in:   DNSRecord{Key: "x.example.com", RecordType: recordTypeCNAME, Value: "y.example.com", TTL: 999999},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.TTLSeconds == nil || *env.TTLSeconds != ttlMaxCNAME {
					t.Errorf("TTLSeconds = %v, want %d", env.TTLSeconds, ttlMaxCNAME)
				}
			},
		},
		{
			name: "TXT has no TTL field",
			in:   DNSRecord{Key: "x.example.com", RecordType: recordTypeTXT, Value: "hi", TTL: 60},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.TTLSeconds != nil {
					t.Errorf("expected no TTLSeconds for TXT, got %d", *env.TTLSeconds)
				}
				if env.Text != "hi" {
					t.Errorf("Text = %q", env.Text)
				}
			},
		},
		{
			name: "MX parses priority and host",
			in:   DNSRecord{Key: "example.com", RecordType: recordTypeMX, Value: "10 mail.example.com"},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.Priority == nil || *env.Priority != 10 {
					t.Errorf("Priority = %v", env.Priority)
				}
				if env.MailServerDomain != "mail.example.com" {
					t.Errorf("MailServerDomain = %q", env.MailServerDomain)
				}
			},
		},
		{
			name:    "MX invalid value errors",
			in:      DNSRecord{Key: "example.com", RecordType: recordTypeMX, Value: "garbage"},
			wantErr: true,
		},
		{
			name: "SRV splits name and target",
			in: DNSRecord{
				Key: "_ldap._tcp.example.com", RecordType: recordTypeSRV,
				Value: "10 20 389 ldap.example.com",
			},
			check: func(t *testing.T, env dnsPolicyEnvelope) {
				t.Helper()
				if env.Service != "_ldap" || env.Protocol != "_tcp" || env.Domain != "example.com" {
					t.Errorf("split = %q/%q/%q", env.Service, env.Protocol, env.Domain)
				}
				if env.Priority == nil || *env.Priority != 10 {
					t.Errorf("Priority = %v", env.Priority)
				}
				if env.ServerDomain != "ldap.example.com" {
					t.Errorf("ServerDomain = %q", env.ServerDomain)
				}
			},
		},
		{
			name: "SRV missing underscored labels errors",
			in: DNSRecord{
				Key: "no.underscore.example.com", RecordType: recordTypeSRV,
				Value: "10 20 389 ldap.example.com",
			},
			wantErr: true,
		},
		{
			name: "SRV malformed value errors",
			in: DNSRecord{
				Key: "_ldap._tcp.example.com", RecordType: recordTypeSRV,
				Value: "not a srv target",
			},
			wantErr: true,
		},
		{
			name:    "unsupported record type errors",
			in:      DNSRecord{Key: "x", RecordType: "NS", Value: "ns1.example.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := fromDNSRecord(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}
			if err != nil {
				t.Fatalf("fromDNSRecord: %v", err)
			}
			if !env.Enabled {
				t.Errorf("Enabled = false, want true")
			}
			if tt.check != nil {
				tt.check(t, env)
			}
		})
	}
}

// TestSRVRoundTrip verifies that toDNSRecord(fromDNSRecord(r)) preserves the
// external-dns record shape.
func TestSRVRoundTrip(t *testing.T) {
	orig := DNSRecord{
		Key: "_ldap._tcp.example.com", RecordType: recordTypeSRV,
		Value: "10 20 389 ldap.example.com",
	}

	env, err := fromDNSRecord(orig)
	if err != nil {
		t.Fatalf("fromDNSRecord: %v", err)
	}

	got, ok := toDNSRecord(env)
	if !ok {
		t.Fatal("toDNSRecord dropped a record it converted from")
	}
	if got.Key != orig.Key {
		t.Errorf("Key roundtrip: got %q want %q", got.Key, orig.Key)
	}
	if got.Value != orig.Value {
		t.Errorf("Value roundtrip: got %q want %q", got.Value, orig.Value)
	}
}

func TestIsSRVConversionError(t *testing.T) {
	tests := []struct {
		name string
		in   DNSRecord
		want bool
	}{
		{
			name: "SRV name without underscores",
			in:   DNSRecord{Key: "no.dots.example.com", RecordType: recordTypeSRV, Value: "10 20 389 host"},
			want: true,
		},
		{
			name: "SRV value malformed",
			in:   DNSRecord{Key: "_x._tcp.example.com", RecordType: recordTypeSRV, Value: "bogus"},
			want: true,
		},
		{
			name: "MX value malformed",
			in:   DNSRecord{Key: "example.com", RecordType: recordTypeMX, Value: "bogus"},
			want: false,
		},
		{
			name: "unsupported record type",
			in:   DNSRecord{Key: "example.com", RecordType: "NS", Value: "ns1.example.com"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fromDNSRecord(tt.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := isSRVConversionError(err); got != tt.want {
				t.Errorf("isSRVConversionError = %v, want %v (err=%v)", got, tt.want, err)
			}
		})
	}
}

func TestClampTTL(t *testing.T) {
	tests := []struct {
		ttl, max, want int
	}{
		{0, 86400, 0},
		{60, 86400, 60},
		{86400, 86400, 86400},
		{86401, 86400, 86400},
		{-5, 86400, 0},
	}
	for _, tt := range tests {
		if got := clampTTL(tt.ttl, tt.max); got != tt.want {
			t.Errorf("clampTTL(%d,%d)=%d, want %d", tt.ttl, tt.max, got, tt.want)
		}
	}
}
