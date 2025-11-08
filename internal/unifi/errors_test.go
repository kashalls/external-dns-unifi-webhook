package unifi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
)

// TestAuthError tests AuthError type
func TestAuthError(t *testing.T) {
	tests := []struct {
		name            string
		operation       string
		status          int
		message         string
		wrappedErr      error
		expectedContain []string
	}{
		{
			name:       "auth error with wrapped error",
			operation:  "login",
			status:     401,
			message:    "invalid credentials",
			wrappedErr: fmt.Errorf("connection timeout"),
			expectedContain: []string{
				"authentication failed during login",
				"status 401",
				"invalid credentials",
				"connection timeout",
			},
		},
		{
			name:       "auth error without wrapped error",
			operation:  "login",
			status:     403,
			message:    "forbidden",
			wrappedErr: nil,
			expectedContain: []string{
				"authentication failed during login",
				"status 403",
				"forbidden",
			},
		},
		{
			name:       "auth error with empty message",
			operation:  "refresh",
			status:     401,
			message:    "",
			wrappedErr: nil,
			expectedContain: []string{
				"authentication failed during refresh",
				"status 401",
			},
		},
		{
			name:       "auth error with status 0",
			operation:  "verify",
			status:     0,
			message:    "no response",
			wrappedErr: nil,
			expectedContain: []string{
				"authentication failed during verify",
				"status 0",
				"no response",
			},
		},
		{
			name:       "auth error with negative status",
			operation:  "test",
			status:     -1,
			message:    "invalid",
			wrappedErr: nil,
			expectedContain: []string{
				"authentication failed during test",
				"status -1",
				"invalid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authErr := &AuthError{
				Operation: tt.operation,
				Status:    tt.status,
				Message:   tt.message,
				Err:       tt.wrappedErr,
			}

			errMsg := authErr.Error()
			for _, expected := range tt.expectedContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("AuthError.Error() = %q, should contain %q", errMsg, expected)
				}
			}

			// Test Unwrap
			unwrapped := authErr.Unwrap()
			if unwrapped != tt.wrappedErr {
				t.Errorf("AuthError.Unwrap() = %v, want %v", unwrapped, tt.wrappedErr)
			}

			// Test that it's compatible with errors.Is when wrapped
			if tt.wrappedErr != nil {
				if !errors.Is(authErr, tt.wrappedErr) {
					t.Errorf("errors.Is(authErr, wrappedErr) = false, want true")
				}
			}
		})
	}
}

// TestNetworkError tests NetworkError type
func TestNetworkError(t *testing.T) {
	tests := []struct {
		name            string
		operation       string
		url             string
		wrappedErr      error
		expectedContain []string
	}{
		{
			name:       "network error with connection timeout",
			operation:  "GET",
			url:        "https://unifi.example.com/api/login",
			wrappedErr: fmt.Errorf("dial tcp: connection timeout"),
			expectedContain: []string{
				"network error during GET",
				"https://unifi.example.com/api/login",
				"dial tcp: connection timeout",
			},
		},
		{
			name:       "network error with DNS failure",
			operation:  "POST",
			url:        "https://invalid.local/api",
			wrappedErr: fmt.Errorf("no such host"),
			expectedContain: []string{
				"network error during POST",
				"https://invalid.local/api",
				"no such host",
			},
		},
		{
			name:       "network error with empty URL",
			operation:  "DELETE",
			url:        "",
			wrappedErr: fmt.Errorf("empty URL"),
			expectedContain: []string{
				"network error during DELETE",
				"empty URL",
			},
		},
		{
			name:       "network error with special characters in URL",
			operation:  "PUT",
			url:        "https://unifi.local/api?param=value&key=секрет",
			wrappedErr: fmt.Errorf("invalid character"),
			expectedContain: []string{
				"network error during PUT",
				"https://unifi.local/api?param=value&key=секрет",
				"invalid character",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			netErr := &NetworkError{
				Operation: tt.operation,
				URL:       tt.url,
				Err:       tt.wrappedErr,
			}

			errMsg := netErr.Error()
			for _, expected := range tt.expectedContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("NetworkError.Error() = %q, should contain %q", errMsg, expected)
				}
			}

			// Test Unwrap
			unwrapped := netErr.Unwrap()
			if unwrapped != tt.wrappedErr {
				t.Errorf("NetworkError.Unwrap() = %v, want %v", unwrapped, tt.wrappedErr)
			}

			// Test that it's compatible with errors.Is
			if !errors.Is(netErr, tt.wrappedErr) {
				t.Errorf("errors.Is(netErr, wrappedErr) = false, want true")
			}
		})
	}
}

// TestAPIError tests APIError type
func TestAPIError(t *testing.T) {
	tests := []struct {
		name            string
		operation       string
		url             string
		statusCode      int
		message         string
		expectedContain []string
	}{
		{
			name:       "API error 404",
			operation:  "GET",
			url:        "https://unifi.local/api/site/default/static-dns/missing",
			statusCode: 404,
			message:    "Record not found",
			expectedContain: []string{
				"API error during GET",
				"https://unifi.local/api/site/default/static-dns/missing",
				"status 404",
				"Record not found",
			},
		},
		{
			name:       "API error 500",
			operation:  "POST",
			url:        "https://unifi.local/api/login",
			statusCode: 500,
			message:    "Internal server error",
			expectedContain: []string{
				"API error during POST",
				"status 500",
				"Internal server error",
			},
		},
		{
			name:       "API error with empty message",
			operation:  "DELETE",
			url:        "https://unifi.local/api/record/123",
			statusCode: 400,
			message:    "",
			expectedContain: []string{
				"API error during DELETE",
				"status 400",
			},
		},
		{
			name:       "API error with status 0",
			operation:  "PATCH",
			url:        "https://unifi.local/api",
			statusCode: 0,
			message:    "Unknown error",
			expectedContain: []string{
				"API error during PATCH",
				"status 0",
				"Unknown error",
			},
		},
		{
			name:       "API error with long message",
			operation:  "POST",
			url:        "https://unifi.local/api/dns",
			statusCode: 422,
			message:    strings.Repeat("Very long error message with lots of details. ", 10),
			expectedContain: []string{
				"API error during POST",
				"status 422",
				"Very long error message",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := &APIError{
				Operation:  tt.operation,
				URL:        tt.url,
				StatusCode: tt.statusCode,
				Message:    tt.message,
			}

			errMsg := apiErr.Error()
			for _, expected := range tt.expectedContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("APIError.Error() = %q, should contain %q", errMsg, expected)
				}
			}
		})
	}
}

// TestDataError tests DataError type
func TestDataError(t *testing.T) {
	tests := []struct {
		name            string
		operation       string
		dataType        string
		wrappedErr      error
		expectedContain []string
	}{
		{
			name:       "data error marshaling JSON",
			operation:  "marshal",
			dataType:   "DNS record",
			wrappedErr: fmt.Errorf("json: unsupported type"),
			expectedContain: []string{
				"data error during marshal",
				"DNS record",
				"json: unsupported type",
			},
		},
		{
			name:       "data error unmarshaling JSON",
			operation:  "unmarshal",
			dataType:   "API response",
			wrappedErr: fmt.Errorf("json: cannot unmarshal"),
			expectedContain: []string{
				"data error during unmarshal",
				"API response",
				"json: cannot unmarshal",
			},
		},
		{
			name:       "data error reading body",
			operation:  "read",
			dataType:   "response body",
			wrappedErr: fmt.Errorf("unexpected EOF"),
			expectedContain: []string{
				"data error during read",
				"response body",
				"unexpected EOF",
			},
		},
		{
			name:       "data error parsing SRV record",
			operation:  "parse",
			dataType:   "SRV record target",
			wrappedErr: fmt.Errorf("invalid format"),
			expectedContain: []string{
				"data error during parse",
				"SRV record target",
				"invalid format",
			},
		},
		{
			name:       "data error with empty dataType",
			operation:  "validate",
			dataType:   "",
			wrappedErr: fmt.Errorf("validation failed"),
			expectedContain: []string{
				"data error during validate",
				"validation failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataErr := &DataError{
				Operation: tt.operation,
				DataType:  tt.dataType,
				Err:       tt.wrappedErr,
			}

			errMsg := dataErr.Error()
			for _, expected := range tt.expectedContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("DataError.Error() = %q, should contain %q", errMsg, expected)
				}
			}

			// Test Unwrap
			unwrapped := dataErr.Unwrap()
			if unwrapped != tt.wrappedErr {
				t.Errorf("DataError.Unwrap() = %v, want %v", unwrapped, tt.wrappedErr)
			}

			// Test that it's compatible with errors.Is
			if !errors.Is(dataErr, tt.wrappedErr) {
				t.Errorf("errors.Is(dataErr, wrappedErr) = false, want true")
			}
		})
	}
}

// TestNewAuthError tests NewAuthError helper
func TestNewAuthError(t *testing.T) {
	wrappedErr := fmt.Errorf("underlying error")
	err := NewAuthError("login", 401, "unauthorized", wrappedErr)

	if err == nil {
		t.Fatal("NewAuthError returned nil")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Fatalf("NewAuthError returned %T, want *AuthError", err)
	}

	if authErr.Operation != "login" {
		t.Errorf("Operation = %q, want %q", authErr.Operation, "login")
	}
	if authErr.Status != 401 {
		t.Errorf("Status = %d, want %d", authErr.Status, 401)
	}
	if authErr.Message != "unauthorized" {
		t.Errorf("Message = %q, want %q", authErr.Message, "unauthorized")
	}
	if authErr.Err != wrappedErr {
		t.Errorf("Err = %v, want %v", authErr.Err, wrappedErr)
	}
}

// TestNewNetworkError tests NewNetworkError helper
func TestNewNetworkError(t *testing.T) {
	wrappedErr := fmt.Errorf("connection refused")
	err := NewNetworkError("POST", "https://example.com", wrappedErr)

	if err == nil {
		t.Fatal("NewNetworkError returned nil")
	}

	netErr, ok := err.(*NetworkError)
	if !ok {
		t.Fatalf("NewNetworkError returned %T, want *NetworkError", err)
	}

	if netErr.Operation != "POST" {
		t.Errorf("Operation = %q, want %q", netErr.Operation, "POST")
	}
	if netErr.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", netErr.URL, "https://example.com")
	}
	if netErr.Err != wrappedErr {
		t.Errorf("Err = %v, want %v", netErr.Err, wrappedErr)
	}
}

// TestNewAPIError tests NewAPIError helper
func TestNewAPIError(t *testing.T) {
	err := NewAPIError("GET", "https://api.example.com", 404, "not found")

	if err == nil {
		t.Fatal("NewAPIError returned nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("NewAPIError returned %T, want *APIError", err)
	}

	if apiErr.Operation != "GET" {
		t.Errorf("Operation = %q, want %q", apiErr.Operation, "GET")
	}
	if apiErr.URL != "https://api.example.com" {
		t.Errorf("URL = %q, want %q", apiErr.URL, "https://api.example.com")
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, 404)
	}
	if apiErr.Message != "not found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "not found")
	}
}

// TestNewDataError tests NewDataError helper
func TestNewDataError(t *testing.T) {
	wrappedErr := fmt.Errorf("json error")
	err := NewDataError("marshal", "user data", wrappedErr)

	if err == nil {
		t.Fatal("NewDataError returned nil")
	}

	dataErr, ok := err.(*DataError)
	if !ok {
		t.Fatalf("NewDataError returned %T, want *DataError", err)
	}

	if dataErr.Operation != "marshal" {
		t.Errorf("Operation = %q, want %q", dataErr.Operation, "marshal")
	}
	if dataErr.DataType != "user data" {
		t.Errorf("DataType = %q, want %q", dataErr.DataType, "user data")
	}
	if dataErr.Err != wrappedErr {
		t.Errorf("Err = %v, want %v", dataErr.Err, wrappedErr)
	}
}

// TestIsAuthError tests IsAuthError helper
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "actual AuthError",
			err:      NewAuthError("login", 401, "fail", nil),
			expected: true,
		},
		{
			name:     "wrapped AuthError",
			err:      errors.Wrap(NewAuthError("login", 401, "fail", nil), "additional context"),
			expected: true,
		},
		{
			name:     "NetworkError",
			err:      NewNetworkError("GET", "url", fmt.Errorf("error")),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthError(tt.err)
			if result != tt.expected {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestIsNetworkError tests IsNetworkError helper
func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "actual NetworkError",
			err:      NewNetworkError("GET", "url", fmt.Errorf("error")),
			expected: true,
		},
		{
			name:     "wrapped NetworkError",
			err:      errors.Wrap(NewNetworkError("GET", "url", fmt.Errorf("error")), "context"),
			expected: true,
		},
		{
			name:     "AuthError",
			err:      NewAuthError("login", 401, "fail", nil),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			if result != tt.expected {
				t.Errorf("IsNetworkError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestIsAPIError tests IsAPIError helper
func TestIsAPIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "actual APIError",
			err:      NewAPIError("GET", "url", 404, "not found"),
			expected: true,
		},
		{
			name:     "wrapped APIError",
			err:      errors.Wrap(NewAPIError("GET", "url", 404, "not found"), "context"),
			expected: true,
		},
		{
			name:     "AuthError",
			err:      NewAuthError("login", 401, "fail", nil),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAPIError(tt.err)
			if result != tt.expected {
				t.Errorf("IsAPIError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestIsDataError tests IsDataError helper
func TestIsDataError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "actual DataError",
			err:      NewDataError("marshal", "data", fmt.Errorf("error")),
			expected: true,
		},
		{
			name:     "wrapped DataError",
			err:      errors.Wrap(NewDataError("marshal", "data", fmt.Errorf("error")), "context"),
			expected: true,
		},
		{
			name:     "AuthError",
			err:      NewAuthError("login", 401, "fail", nil),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDataError(tt.err)
			if result != tt.expected {
				t.Errorf("IsDataError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestErrorChaining tests error chaining with multiple wraps
func TestErrorChaining(t *testing.T) {
	baseErr := fmt.Errorf("root cause")
	dataErr := NewDataError("parse", "config", baseErr)
	wrappedOnce := errors.Wrap(dataErr, "first wrap")
	wrappedTwice := errors.Wrap(wrappedOnce, "second wrap")

	// Should still be detectable as DataError
	if !IsDataError(wrappedTwice) {
		t.Error("IsDataError failed to detect error through multiple wraps")
	}

	// Should be able to unwrap to base error
	if !errors.Is(wrappedTwice, baseErr) {
		t.Error("errors.Is failed to match base error through chain")
	}

	// Error message should contain context from wraps
	errMsg := wrappedTwice.Error()
	if !strings.Contains(errMsg, "second wrap") {
		t.Errorf("error message missing wrap context: %s", errMsg)
	}
}

// TestErrorAs tests errors.As with custom error types
func TestErrorAs(t *testing.T) {
	authErr := NewAuthError("login", 401, "unauthorized", nil)
	wrappedErr := errors.Wrap(authErr, "wrapped")

	var targetErr *AuthError
	if !errors.As(wrappedErr, &targetErr) {
		t.Fatal("errors.As failed to extract AuthError")
	}

	if targetErr.Operation != "login" {
		t.Errorf("extracted AuthError has Operation = %q, want %q", targetErr.Operation, "login")
	}
	if targetErr.Status != 401 {
		t.Errorf("extracted AuthError has Status = %d, want %d", targetErr.Status, 401)
	}
}
