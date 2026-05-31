package unifi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestNetworkError tests NetworkError type.
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
			operation:  http.MethodGet,
			url:        testURLLoginExample,
			wrappedErr: errors.New("dial tcp: connection timeout"),
			expectedContain: []string{
				"network error during GET",
				testURLLoginExample,
				"dial tcp: connection timeout",
			},
		},
		{
			name:       "network error with DNS failure",
			operation:  http.MethodPost,
			url:        testURLInvalid,
			wrappedErr: errors.New("no such host"),
			expectedContain: []string{
				"network error during POST",
				testURLInvalid,
				"no such host",
			},
		},
		{
			name:       "network error with empty URL",
			operation:  http.MethodDelete,
			url:        "",
			wrappedErr: errors.New("empty URL"),
			expectedContain: []string{
				"network error during DELETE",
				"empty URL",
			},
		},
		{
			name:       "network error with special characters in URL",
			operation:  "PUT",
			url:        testURLSpecialChars,
			wrappedErr: errors.New("invalid character"),
			expectedContain: []string{
				"network error during PUT",
				testURLSpecialChars,
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
			if !errors.Is(unwrapped, tt.wrappedErr) {
				t.Errorf("NetworkError.Unwrap() = %v, want %v", unwrapped, tt.wrappedErr)
			}

			// Test that it's compatible with errors.Is
			if !errors.Is(netErr, tt.wrappedErr) {
				t.Errorf("errors.Is(netErr, wrappedErr) = false, want true")
			}
		})
	}
}

// TestAPIError tests APIError type.
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
			operation:  http.MethodGet,
			url:        testURLMissingRecord,
			statusCode: 404,
			message:    testMsgRecordNotFound,
			expectedContain: []string{
				"API error during GET",
				testURLMissingRecord,
				"status 404",
				testMsgRecordNotFound,
			},
		},
		{
			name:       "API error 500",
			operation:  http.MethodPost,
			url:        "https://unifi.local/api/login",
			statusCode: 500,
			message:    testMsgInternalServer,
			expectedContain: []string{
				testAPIErrPost,
				"status 500",
				testMsgInternalServer,
			},
		},
		{
			name:       "API error with empty message",
			operation:  http.MethodDelete,
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
			message:    testMsgUnknownError,
			expectedContain: []string{
				"API error during PATCH",
				testStatus0,
				testMsgUnknownError,
			},
		},
		{
			name:       "API error with long message",
			operation:  http.MethodPost,
			url:        "https://unifi.local/api/dns",
			statusCode: 422,
			message:    strings.Repeat("Very long error message with lots of details. ", 10),
			expectedContain: []string{
				testAPIErrPost,
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

// TestDataError tests DataError type.
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
			operation:  testOpMarshal,
			dataType:   testDataDNSRecord,
			wrappedErr: errors.New("json: unsupported type"),
			expectedContain: []string{
				"data error during marshal",
				testDataDNSRecord,
				"json: unsupported type",
			},
		},
		{
			name:       "data error unmarshaling JSON",
			operation:  "unmarshal",
			dataType:   testDataAPIResp,
			wrappedErr: errors.New("json: cannot unmarshal"),
			expectedContain: []string{
				"data error during unmarshal",
				testDataAPIResp,
				"json: cannot unmarshal",
			},
		},
		{
			name:       "data error reading body",
			operation:  "read",
			dataType:   testDataRespBody,
			wrappedErr: errors.New("unexpected EOF"),
			expectedContain: []string{
				"data error during read",
				testDataRespBody,
				"unexpected EOF",
			},
		},
		{
			name:       "data error parsing SRV record",
			operation:  "parse",
			dataType:   testDataSRVTarget,
			wrappedErr: errors.New("invalid format"),
			expectedContain: []string{
				"data error during parse",
				testDataSRVTarget,
				"invalid format",
			},
		},
		{
			name:       "data error with empty dataType",
			operation:  "validate",
			dataType:   "",
			wrappedErr: errors.New("validation failed"),
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
			if !errors.Is(unwrapped, tt.wrappedErr) {
				t.Errorf("DataError.Unwrap() = %v, want %v", unwrapped, tt.wrappedErr)
			}

			// Test that it's compatible with errors.Is
			if !errors.Is(dataErr, tt.wrappedErr) {
				t.Errorf("errors.Is(dataErr, wrappedErr) = false, want true")
			}
		})
	}
}

// TestNewNetworkError tests NewNetworkError helper.
func TestNewNetworkError(t *testing.T) {
	wrappedErr := errors.New("connection refused")
	err := NewNetworkError(http.MethodPost, testURLExample, wrappedErr)

	if err == nil {
		t.Fatal("NewNetworkError returned nil")
	}

	netErr := &NetworkError{}
	ok := errors.As(err, &netErr)
	if !ok {
		t.Fatalf("NewNetworkError returned %T, want *NetworkError", err)
	}

	if netErr.Operation != http.MethodPost {
		t.Errorf("Operation = %q, want %q", netErr.Operation, http.MethodPost)
	}
	if netErr.URL != testURLExample {
		t.Errorf("URL = %q, want %q", netErr.URL, testURLExample)
	}
	if !errors.Is(netErr.Err, wrappedErr) {
		t.Errorf("Err = %v, want %v", netErr.Err, wrappedErr)
	}
}

// TestNewAPIError tests NewAPIError helper.
func TestNewAPIError(t *testing.T) {
	err := NewAPIError(http.MethodGet, "https://api.example.com", 404, "not found")

	if err == nil {
		t.Fatal("NewAPIError returned nil")
	}

	apiErr := &APIError{}
	ok := errors.As(err, &apiErr)
	if !ok {
		t.Fatalf("NewAPIError returned %T, want *APIError", err)
	}

	if apiErr.Operation != http.MethodGet {
		t.Errorf("Operation = %q, want %q", apiErr.Operation, http.MethodGet)
	}
	if apiErr.URL != "https://api.example.com" {
		t.Errorf("URL = %q, want %q", apiErr.URL, "https://api.example.com")
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, 404)
	}
	if apiErr.Message != "not found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "not found")
	}
}

// TestNewDataError tests NewDataError helper.
func TestNewDataError(t *testing.T) {
	wrappedErr := errors.New("json error")
	err := NewDataError(testOpMarshal, "user data", wrappedErr)

	if err == nil {
		t.Fatal("NewDataError returned nil")
	}

	dataErr := &DataError{}
	ok := errors.As(err, &dataErr)
	if !ok {
		t.Fatalf("NewDataError returned %T, want *DataError", err)
	}

	if dataErr.Operation != testOpMarshal {
		t.Errorf("Operation = %q, want %q", dataErr.Operation, testOpMarshal)
	}
	if dataErr.DataType != "user data" {
		t.Errorf("DataType = %q, want %q", dataErr.DataType, "user data")
	}
	if !errors.Is(dataErr.Err, wrappedErr) {
		t.Errorf("Err = %v, want %v", dataErr.Err, wrappedErr)
	}
}

// TestIsNetworkError tests IsNetworkError helper.
func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "actual NetworkError",
			err:      NewNetworkError(http.MethodGet, "url", errors.New("error")),
			expected: true,
		},
		{
			name:     "wrapped NetworkError",
			err:      fmt.Errorf("context: %w", NewNetworkError(http.MethodGet, "url", errors.New("error"))),
			expected: true,
		},
		{
			name:     "APIError",
			err:      NewAPIError(http.MethodGet, "url", 404, "not found"),
			expected: false,
		},
		{
			name:     testMsgGenericError,
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     testNameNilError,
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

// TestErrorChaining tests error chaining with multiple wraps.
func TestErrorChaining(t *testing.T) {
	baseErr := errors.New("root cause")
	dataErr := NewDataError("parse", "config", baseErr)
	wrappedOnce := fmt.Errorf("first wrap: %w", dataErr)
	wrappedTwice := fmt.Errorf("second wrap: %w", wrappedOnce)

	// Should still be detectable as DataError through multiple wraps.
	var target *DataError
	if !errors.As(wrappedTwice, &target) {
		t.Error("errors.As failed to detect DataError through multiple wraps")
	}

	// Should be able to unwrap to base error.
	if !errors.Is(wrappedTwice, baseErr) {
		t.Error("errors.Is failed to match base error through chain")
	}

	// Error message should contain context from wraps.
	errMsg := wrappedTwice.Error()
	if !strings.Contains(errMsg, "second wrap") {
		t.Errorf("error message missing wrap context: %s", errMsg)
	}
}

// TestErrorAs tests errors.As with custom error types. APIError carries the
// status code that DeleteRecord relies on to treat a 404 as success.
func TestErrorAs(t *testing.T) {
	apiErr := NewAPIError(http.MethodGet, testURLExample, http.StatusNotFound, "not found")
	wrappedErr := fmt.Errorf("wrapped: %w", apiErr)

	var targetErr *APIError
	if !errors.As(wrappedErr, &targetErr) {
		t.Fatal("errors.As failed to extract APIError")
	}

	if targetErr.Operation != http.MethodGet {
		t.Errorf("extracted APIError has Operation = %q, want %q", targetErr.Operation, http.MethodGet)
	}
	if targetErr.StatusCode != http.StatusNotFound {
		t.Errorf("extracted APIError has StatusCode = %d, want %d", targetErr.StatusCode, http.StatusNotFound)
	}
}
