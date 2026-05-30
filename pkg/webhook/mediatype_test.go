package webhook

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
)

// Shared test fixtures, declared as constants to satisfy the goconst linter.
const (
	mtV1              = "application/external.dns.webhook+json;version=1"
	mtV2              = "application/external.dns.webhook+json;version=2"
	mtAppJSON         = "application/json"
	mtV1Spaced        = " application/external.dns.webhook+json;version=1 "
	mtV1Upper         = "Application/external.dns.webhook+json;version=1"
	errMsgUnsupported = "unsupported media type version"
)

func TestMediaTypeVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected mediaType
	}{
		{
			name:     "version 1",
			version:  "1",
			expected: mtV1,
		},
		{
			name:     "version 2",
			version:  "2",
			expected: mtV2,
		},
		{
			name:     "empty version",
			version:  "",
			expected: "application/external.dns.webhook+json;version=",
		},
		{
			name:     "special characters",
			version:  "v1.0-beta",
			expected: "application/external.dns.webhook+json;version=v1.0-beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mediaTypeVersion(tt.version)
			if result != tt.expected {
				t.Errorf("mediaTypeVersion(%q) = %q, want %q", tt.version, result, tt.expected)
			}
		})
	}
}

func TestMediaType_Is(t *testing.T) {
	tests := []struct {
		name        string
		mediaType   mediaType
		headerValue string
		expected    bool
	}{
		{
			name:        "exact match",
			mediaType:   mtV1,
			headerValue: mtV1,
			expected:    true,
		},
		{
			name:        "no match - different version",
			mediaType:   mtV1,
			headerValue: mtV2,
			expected:    false,
		},
		{
			name:        "no match - missing version",
			mediaType:   mtV1,
			headerValue: mediaTypeFormat,
			expected:    false,
		},
		{
			name:        "no match - empty string",
			mediaType:   mtV1,
			headerValue: "",
			expected:    false,
		},
		{
			name:        "no match - completely different",
			mediaType:   mtV1,
			headerValue: mtAppJSON,
			expected:    false,
		},
		{
			name:        "no match - extra whitespace",
			mediaType:   mtV1,
			headerValue: mtV1Spaced,
			expected:    false,
		},
		{
			name:        "no match - case difference",
			mediaType:   mtV1,
			headerValue: mtV1Upper,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mediaType.Is(tt.headerValue)
			if result != tt.expected {
				t.Errorf("mediaType.Is(%q) = %v, want %v", tt.headerValue, result, tt.expected)
			}
		})
	}
}

func TestCheckAndGetMediaTypeHeaderValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantVersion string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid version 1",
			value:       mtV1,
			wantVersion: "1",
			wantErr:     false,
		},
		{
			name:        "unsupported version 2",
			value:       mtV2,
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "unsupported version 0",
			value:       "application/external.dns.webhook+json;version=0",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "missing version parameter",
			value:       mediaTypeFormat,
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "empty string",
			value:       "",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "completely wrong media type",
			value:       mtAppJSON,
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "wrong format with version",
			value:       "application/json;version=1",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "extra whitespace",
			value:       mtV1Spaced,
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "case sensitive - uppercase",
			value:       mtV1Upper,
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "malformed - missing semicolon",
			value:       "application/external.dns.webhook+jsonversion=1",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "malformed - extra parameters",
			value:       "application/external.dns.webhook+json;version=1;charset=utf-8",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "version with leading zero",
			value:       "application/external.dns.webhook+json;version=01",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "negative version",
			value:       "application/external.dns.webhook+json;version=-1",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
		{
			name:        "decimal version",
			value:       "application/external.dns.webhook+json;version=1.0",
			wantVersion: "",
			wantErr:     true,
			errContains: errMsgUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := checkAndGetMediaTypeHeaderValue(tt.value)

			if tt.wantErr {
				if err == nil {
					t.Errorf("checkAndGetMediaTypeHeaderValue(%q) error = nil, wantErr %v", tt.value, tt.wantErr)

					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("checkAndGetMediaTypeHeaderValue(%q) error = %v, should contain %q", tt.value, err, tt.errContains)
				}
				// Check that error can be unwrapped to errUnsupportedMediaType
				if !errors.Is(err, errUnsupportedMediaType) {
					t.Errorf("checkAndGetMediaTypeHeaderValue(%q) error should wrap errUnsupportedMediaType", tt.value)
				}
			} else if err != nil {
				t.Errorf("checkAndGetMediaTypeHeaderValue(%q) unexpected error = %v", tt.value, err)

				return
			}

			if version != tt.wantVersion {
				t.Errorf("checkAndGetMediaTypeHeaderValue(%q) version = %q, want %q", tt.value, version, tt.wantVersion)
			}
		})
	}
}

func TestMediaTypeVersion1Constant(t *testing.T) {
	expected := mediaType(mtV1)
	if mediaTypeVersion1 != expected {
		t.Errorf("mediaTypeVersion1 = %q, want %q", mediaTypeVersion1, expected)
	}
}

func TestErrUnsupportedMediaType(t *testing.T) {
	if errUnsupportedMediaType == nil {
		t.Error("errUnsupportedMediaType should not be nil")
	}

	expected := errMsgUnsupported
	if errUnsupportedMediaType.Error() != expected {
		t.Errorf("errUnsupportedMediaType.Error() = %q, want %q", errUnsupportedMediaType.Error(), expected)
	}
}

// TestCheckAndGetMediaTypeHeaderValueErrorMessage verifies error message format.
func TestCheckAndGetMediaTypeHeaderValueErrorMessage(t *testing.T) {
	value := mtAppJSON
	_, err := checkAndGetMediaTypeHeaderValue(value)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()

	// Error should mention the received value
	if !strings.Contains(errMsg, value) {
		t.Errorf("error message should contain received value %q, got: %s", value, errMsg)
	}

	// Error should mention supported media types
	if !strings.Contains(errMsg, mtV1) {
		t.Errorf("error message should contain supported media type, got: %s", errMsg)
	}

	// Error should mention "supported media types are"
	if !strings.Contains(errMsg, "supported media types are") {
		t.Errorf("error message should contain 'supported media types are', got: %s", errMsg)
	}
}
