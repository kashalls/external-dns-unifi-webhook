package unifi

import (
	"errors"
	"fmt"
)

// NetworkError represents network-related errors.
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

// APIError represents UniFi API errors.
type APIError struct {
	Operation  string
	StatusCode int
	Message    string
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error during %s to %s (status %d): %s", e.Operation, e.URL, e.StatusCode, e.Message)
}

// DataError represents data marshaling/unmarshaling errors.
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

// NewNetworkError creates a new network error.
func NewNetworkError(operation, url string, err error) error {
	return &NetworkError{Operation: operation, URL: url, Err: err}
}

// NewAPIError creates a new API error.
func NewAPIError(operation, url string, statusCode int, message string) error {
	return &APIError{Operation: operation, URL: url, StatusCode: statusCode, Message: message}
}

// NewDataError creates a new data error.
func NewDataError(operation, dataType string, err error) error {
	return &DataError{Operation: operation, DataType: dataType, Err: err}
}

// IsNetworkError reports whether err is a *NetworkError.
func IsNetworkError(err error) bool {
	_, ok := errors.AsType[*NetworkError](err)

	return ok
}
