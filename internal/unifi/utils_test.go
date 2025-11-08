package unifi

import (
	"testing"
)

func TestFormatUrl(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "login path",
			path:     "%s/api/auth/login",
			params:   []string{"https://unifi.local"},
			expected: "https://unifi.local/api/auth/login",
		},
		{
			name:     "records path with site",
			path:     "%s/proxy/network/v2/api/site/%s/static-dns/%s",
			params:   []string{"https://unifi.local", "default"},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/",
		},
		{
			name:     "records path with site and record ID",
			path:     "%s/proxy/network/v2/api/site/%s/static-dns/%s",
			params:   []string{"https://unifi.local", "default", "abc123"},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/abc123",
		},
		{
			name:     "external controller login",
			path:     "%s/api/login",
			params:   []string{"https://ui.com"},
			expected: "https://ui.com/api/login",
		},
		{
			name:     "external controller records",
			path:     "%s/v2/api/site/%s/static-dns/%s",
			params:   []string{"https://ui.com", "site-id", "record-id"},
			expected: "https://ui.com/v2/api/site/site-id/static-dns/record-id",
		},
		{
			name:     "no placeholders - appends params",
			path:     "/api/login",
			params:   []string{"https://example.com"},
			expected: "/api/loginhttps://example.com",
		},
		{
			name:     "empty params",
			path:     "%s/api/%s",
			params:   []string{},
			expected: "/api/",
		},
		{
			name:     "empty string params",
			path:     "%s/api/%s/data",
			params:   []string{"https://example.com", ""},
			expected: "https://example.com/api//data",
		},
		{
			name:     "more placeholders than params",
			path:     "%s/api/%s/site/%s",
			params:   []string{"https://example.com"},
			expected: "https://example.com/api//site/",
		},
		{
			name:     "URL with port",
			path:     "%s/api/login",
			params:   []string{"https://unifi.local:8443"},
			expected: "https://unifi.local:8443/api/login",
		},
		{
			name:     "URL with query params",
			path:     "%s/api/site/%s?filter=active",
			params:   []string{"https://unifi.local", "default"},
			expected: "https://unifi.local/api/site/default?filter=active",
		},
		{
			name:     "special characters in params",
			path:     "%s/api/%s",
			params:   []string{"https://example.com", "special-chars_123"},
			expected: "https://example.com/api/special-chars_123",
		},
		{
			name:     "unicode in params",
			path:     "%s/api/%s",
			params:   []string{"https://пример.рф", "сайт"},
			expected: "https://пример.рф/api/сайт",
		},
		{
			name:     "empty path - appends params",
			path:     "",
			params:   []string{"https://example.com"},
			expected: "https://example.com",
		},
		{
			name:     "single placeholder",
			path:     "%s",
			params:   []string{"https://example.com"},
			expected: "https://example.com",
		},
		{
			name:     "consecutive placeholders",
			path:     "%s%s%s",
			params:   []string{"a", "b", "c"},
			expected: "abc",
		},
		{
			name:     "path with trailing slash",
			path:     "%s/api/%s/",
			params:   []string{"https://example.com", "v1"},
			expected: "https://example.com/api/v1/",
		},
		{
			name:     "path with multiple slashes",
			path:     "%s//api//%s",
			params:   []string{"https://example.com", "endpoint"},
			expected: "https://example.com//api//endpoint",
		},
		{
			name:     "nil params equivalent",
			path:     "%s/api/%s",
			params:   nil,
			expected: "/api/",
		},
		{
			name:     "IPv4 address",
			path:     "%s/api/login",
			params:   []string{"https://192.168.1.1"},
			expected: "https://192.168.1.1/api/login",
		},
		{
			name:     "IPv6 address",
			path:     "%s/api/login",
			params:   []string{"https://[2001:db8::1]"},
			expected: "https://[2001:db8::1]/api/login",
		},
		{
			name:     "path injection attempt",
			path:     "%s/api/%s",
			params:   []string{"https://example.com", "../../../etc/passwd"},
			expected: "https://example.com/api/../../../etc/passwd",
		},
		{
			name:     "URL with fragment",
			path:     "%s/api/%s#section",
			params:   []string{"https://example.com", "resource"},
			expected: "https://example.com/api/resource#section",
		},
		{
			name:     "URL with credentials",
			path:     "%s/api",
			params:   []string{"https://user:pass@example.com"},
			expected: "https://user:pass@example.com/api",
		},
		{
			name:     "long record ID",
			path:     "%s/api/site/%s/static-dns/%s",
			params:   []string{"https://example.com", "default", "abcdef0123456789abcdef0123456789abcdef01"},
			expected: "https://example.com/api/site/default/static-dns/abcdef0123456789abcdef0123456789abcdef01",
		},
		{
			name:     "whitespace in params",
			path:     "%s/api/%s",
			params:   []string{"https://example.com", "my site"},
			expected: "https://example.com/api/my site",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUrl(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatUrl(%q, %v) = %q, want %q", tt.path, tt.params, result, tt.expected)
			}
		})
	}
}

// TestFormatUrlRealWorldUsage tests actual usage patterns from the codebase.
func TestFormatUrlRealWorldUsage(t *testing.T) {
	const (
		host                    = "https://unifi.local"
		site                    = "default"
		recordID                = "507f1f77bcf86cd799439011"
		unifiLoginPath          = "%s/api/auth/login"
		unifiLoginPathExternal  = "%s/api/login"
		unifiRecordPath         = "%s/proxy/network/v2/api/site/%s/static-dns/%s"
		unifiRecordPathExternal = "%s/v2/api/site/%s/static-dns/%s"
	)

	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "internal controller login",
			path:     unifiLoginPath,
			params:   []string{host},
			expected: "https://unifi.local/api/auth/login",
		},
		{
			name:     "external controller login",
			path:     unifiLoginPathExternal,
			params:   []string{"https://ui.com"},
			expected: "https://ui.com/api/login",
		},
		{
			name:     "get all records (internal)",
			path:     unifiRecordPath,
			params:   []string{host, site},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/",
		},
		{
			name:     "get specific record (internal)",
			path:     unifiRecordPath,
			params:   []string{host, site, recordID},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/507f1f77bcf86cd799439011",
		},
		{
			name:     "get all records (external)",
			path:     unifiRecordPathExternal,
			params:   []string{"https://ui.com", site},
			expected: "https://ui.com/v2/api/site/default/static-dns/",
		},
		{
			name:     "get specific record (external)",
			path:     unifiRecordPathExternal,
			params:   []string{"https://ui.com", site, recordID},
			expected: "https://ui.com/v2/api/site/default/static-dns/507f1f77bcf86cd799439011",
		},
		{
			name:     "custom site name",
			path:     unifiRecordPath,
			params:   []string{host, "my-custom-site"},
			expected: "https://unifi.local/proxy/network/v2/api/site/my-custom-site/static-dns/",
		},
		{
			name:     "controller with port",
			path:     unifiLoginPath,
			params:   []string{"https://192.168.1.1:8443"},
			expected: "https://192.168.1.1:8443/api/auth/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUrl(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatUrl() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestFormatUrlEdgeCases tests boundary conditions.
func TestFormatUrlEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "extremely long URL",
			path:     "%s/api/%s",
			params:   []string{"https://example.com", string(make([]byte, 10000))},
			expected: "https://example.com/api/" + string(make([]byte, 10000)),
		},
		{
			name:     "many placeholders",
			path:     "%s/%s/%s/%s/%s/%s/%s/%s/%s/%s",
			params:   []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			expected: "a/b/c/d/e/f/g/h/i/j",
		},
		{
			name:     "only placeholders",
			path:     "%s%s%s",
			params:   []string{"", "", ""},
			expected: "",
		},
		{
			name:     "escaped percent (not placeholder) - treats %% as single % and s as literal",
			path:     "%%s/api",
			params:   []string{"value"},
			expected: "%value/api",
		},
		{
			name:     "mixed content",
			path:     "prefix%smiddle%ssuffix",
			params:   []string{"AAA", "BBB"},
			expected: "prefixAAAmiddleBBBsuffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUrl(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatUrl() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestFormatUrlPanic tests that function panics when params exceed segments.
func TestFormatUrlPanic(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		params []string
	}{
		{
			name:   "more params than placeholders - causes panic",
			path:   "%s/api",
			params: []string{"https://example.com", "extra", "params"},
		},
		{
			name:   "three extra params",
			path:   "%s",
			params: []string{"a", "b", "c", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("FormatUrl(%q, %v) did not panic", tt.path, tt.params)
				}
			}()
			_ = FormatUrl(tt.path, tt.params...)
		})
	}
}
