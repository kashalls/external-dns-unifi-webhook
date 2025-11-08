package unifi

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// Error types for different categories of failures

// AuthError represents authentication-related errors
type AuthError struct {
	Operation string
	Status    int
	Message   string
	Err       error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("authentication failed during %s (status %d): %s: %v", e.Operation, e.Status, e.Message, e.Err)
	}
	return fmt.Sprintf("authentication failed during %s (status %d): %s", e.Operation, e.Status, e.Message)
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// NetworkError represents network-related errors
type NetworkError struct {
	Operation string
	URL       string
	Err       error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error during %s to %s: %v", e.Operation, e.URL, e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// APIError represents UniFi API errors
type APIError struct {
	Operation  string
	StatusCode int
	Message    string
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error during %s to %s (status %d): %s", e.Operation, e.URL, e.StatusCode, e.Message)
}

// DataError represents data marshaling/unmarshaling errors
type DataError struct {
	Operation string
	DataType  string
	Err       error
}

func (e *DataError) Error() string {
	return fmt.Sprintf("data error during %s of %s: %v", e.Operation, e.DataType, e.Err)
}

func (e *DataError) Unwrap() error {
	return e.Err
}

// Helper functions for creating typed errors

// NewAuthError creates a new authentication error
func NewAuthError(operation string, status int, message string, err error) error {
	return &AuthError{
		Operation: operation,
		Status:    status,
		Message:   message,
		Err:       err,
	}
}

// NewNetworkError creates a new network error
func NewNetworkError(operation, url string, err error) error {
	return &NetworkError{
		Operation: operation,
		URL:       url,
		Err:       err,
	}
}

// NewAPIError creates a new API error
func NewAPIError(operation, url string, statusCode int, message string) error {
	return &APIError{
		Operation:  operation,
		URL:        url,
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewDataError creates a new data error
func NewDataError(operation, dataType string, err error) error {
	return &DataError{
		Operation: operation,
		DataType:  dataType,
		Err:       err,
	}
}

// Type checking helpers

// IsAuthError checks if error is an authentication error
func IsAuthError(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}

// IsNetworkError checks if error is a network error
func IsNetworkError(err error) bool {
	var netErr *NetworkError
	return errors.As(err, &netErr)
}

// IsAPIError checks if error is an API error
func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}

// IsDataError checks if error is a data error
func IsDataError(err error) bool {
	var dataErr *DataError
	return errors.As(err, &dataErr)
}
