package unifi

import (
	"testing"
)

func TestFormatURL(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "login path",
			path:     "%s/api/auth/login",
			params:   []string{testHost},
			expected: testURLLoginInternal,
		},
		{
			name:     "records path with site",
			path:     unifiRecordPath,
			params:   []string{testHost, testSite},
			expected: testURLRecordsInternal,
		},
		{
			name:     "records path with site and record ID",
			path:     unifiRecordPath,
			params:   []string{testHost, testSite, "abc123"},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/abc123",
		},
		{
			name:     "external controller login",
			path:     unifiLoginPathExternal,
			params:   []string{testHostExt},
			expected: "https://ui.com/api/login",
		},
		{
			name:     "external controller records",
			path:     unifiRecordPathExternal,
			params:   []string{testHostExt, "site-id", "record-id"},
			expected: "https://ui.com/v2/api/site/site-id/static-dns/record-id",
		},
		{
			name:     "no placeholders - appends params",
			path:     "/api/login",
			params:   []string{testURLExample},
			expected: "/api/loginhttps://example.com",
		},
		{
			name:     "empty params",
			path:     testPathHostParam,
			params:   []string{},
			expected: testPathAPISlash,
		},
		{
			name:     "empty string params",
			path:     "%s/api/%s/data",
			params:   []string{testURLExample, ""},
			expected: "https://example.com/api//data",
		},
		{
			name:     "more placeholders than params",
			path:     "%s/api/%s/site/%s",
			params:   []string{testURLExample},
			expected: "https://example.com/api//site/",
		},
		{
			name:     "URL with port",
			path:     unifiLoginPathExternal,
			params:   []string{"https://unifi.local:8443"},
			expected: "https://unifi.local:8443/api/login",
		},
		{
			name:     "URL with query params",
			path:     "%s/api/site/%s?filter=active",
			params:   []string{testHost, testSite},
			expected: "https://unifi.local/api/site/default?filter=active",
		},
		{
			name:     "special characters in params",
			path:     testPathHostParam,
			params:   []string{testURLExample, "special-chars_123"},
			expected: "https://example.com/api/special-chars_123",
		},
		{
			name:     "unicode in params",
			path:     testPathHostParam,
			params:   []string{"https://пример.рф", "сайт"},
			expected: "https://пример.рф/api/сайт",
		},
		{
			name:     "empty path - appends params",
			path:     "",
			params:   []string{testURLExample},
			expected: testURLExample,
		},
		{
			name:     "single placeholder",
			path:     testPathSingle,
			params:   []string{testURLExample},
			expected: testURLExample,
		},
		{
			name:     "consecutive placeholders",
			path:     testPathTriple,
			params:   []string{"a", "b", "c"},
			expected: "abc",
		},
		{
			name:     "path with trailing slash",
			path:     "%s/api/%s/",
			params:   []string{testURLExample, "v1"},
			expected: "https://example.com/api/v1/",
		},
		{
			name:     "path with multiple slashes",
			path:     "%s//api//%s",
			params:   []string{testURLExample, "endpoint"},
			expected: "https://example.com//api//endpoint",
		},
		{
			name:     "nil params equivalent",
			path:     testPathHostParam,
			params:   nil,
			expected: testPathAPISlash,
		},
		{
			name:     "IPv4 address",
			path:     unifiLoginPathExternal,
			params:   []string{"https://192.168.1.1"},
			expected: "https://192.168.1.1/api/login",
		},
		{
			name:     "IPv6 address",
			path:     unifiLoginPathExternal,
			params:   []string{"https://[2001:db8::1]"},
			expected: "https://[2001:db8::1]/api/login",
		},
		{
			name:     "path injection attempt",
			path:     testPathHostParam,
			params:   []string{testURLExample, "../../../etc/passwd"},
			expected: "https://example.com/api/../../../etc/passwd",
		},
		{
			name:     "URL with fragment",
			path:     "%s/api/%s#section",
			params:   []string{testURLExample, "resource"},
			expected: "https://example.com/api/resource#section",
		},
		{
			name:     "URL with credentials",
			path:     testPathHost,
			params:   []string{testURLCreds},
			expected: testURLCreds + "/api",
		},
		{
			name:     "long record ID",
			path:     "%s/api/site/%s/static-dns/%s",
			params:   []string{testURLExample, testSite, "abcdef0123456789abcdef0123456789abcdef01"},
			expected: "https://example.com/api/site/default/static-dns/abcdef0123456789abcdef0123456789abcdef01",
		},
		{
			name:     "whitespace in params",
			path:     testPathHostParam,
			params:   []string{testURLExample, "my site"},
			expected: "https://example.com/api/my site",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatURL(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatURL(%q, %v) = %q, want %q", tt.path, tt.params, result, tt.expected)
			}
		})
	}
}

// TestFormatURLRealWorldUsage tests actual usage patterns from the codebase.
func TestFormatURLRealWorldUsage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "internal controller login",
			path:     unifiLoginPath,
			params:   []string{testHost},
			expected: testURLLoginInternal,
		},
		{
			name:     testNameExtCtrlLogin,
			path:     unifiLoginPathExternal,
			params:   []string{testHostExt},
			expected: testURLLoginExternal,
		},
		{
			name:     "get all records (internal)",
			path:     unifiRecordPath,
			params:   []string{testHost, testSite},
			expected: testURLRecordsInternal,
		},
		{
			name:     "get specific record (internal)",
			path:     unifiRecordPath,
			params:   []string{testHost, testSite, testRecordID},
			expected: "https://unifi.local/proxy/network/v2/api/site/default/static-dns/507f1f77bcf86cd799439011",
		},
		{
			name:     "get all records (external)",
			path:     unifiRecordPathExternal,
			params:   []string{testHostExt, testSite},
			expected: "https://ui.com/v2/api/site/default/static-dns/",
		},
		{
			name:     "get specific record (external)",
			path:     unifiRecordPathExternal,
			params:   []string{testHostExt, testSite, testRecordID},
			expected: "https://ui.com/v2/api/site/default/static-dns/507f1f77bcf86cd799439011",
		},
		{
			name:     "custom site name",
			path:     unifiRecordPath,
			params:   []string{testHost, "my-custom-site"},
			expected: "https://unifi.local/proxy/network/v2/api/site/my-custom-site/static-dns/",
		},
		{
			name:     "controller with port",
			path:     unifiLoginPath,
			params:   []string{testHostPort},
			expected: "https://192.168.1.1:8443/api/auth/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatURL(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestFormatURLEdgeCases tests boundary conditions.
func TestFormatURLEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []string
		expected string
	}{
		{
			name:     "extremely long URL",
			path:     testPathHostParam,
			params:   []string{testURLExample, string(make([]byte, 10000))},
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
			path:     testPathTriple,
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
			result := FormatURL(tt.path, tt.params...)
			if result != tt.expected {
				t.Errorf("FormatURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestFormatURLPanic tests that function panics when params exceed segments.
func TestFormatURLPanic(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		params []string
	}{
		{
			name:   "more params than placeholders - causes panic",
			path:   "%s/api",
			params: []string{testURLExample, "extra", "params"},
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
					t.Errorf("FormatURL(%q, %v) did not panic", tt.path, tt.params)
				}
			}()
			_ = FormatURL(tt.path, tt.params...)
		})
	}
}
