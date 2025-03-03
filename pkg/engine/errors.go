package engine

import (
	"errors"
	"fmt"
	"net/http"
)

// Common engine errors.
var (
	ErrEngineNotInitialized = errors.New("engine is not initialized")
	ErrFunctionNotLoaded    = errors.New("function is not loaded")
	ErrFunctionNotFound     = errors.New("function not found")
)

// Error represents an internal engine error.
type Error struct {
	message string
	cause   error
}

// Error implements the error interface.
func (e Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Unwrap implements errors.Unwrap for Go 1.13+ error wrapping.
func (e Error) Unwrap() error {
	return e.cause
}

// NewEngineError creates a new engine error with the given message.
func NewEngineError(message string) error {
	return Error{message: message}
}

// WrapEngineError wraps an error with an engine error.
func WrapEngineError(message string, err error) error {
	return Error{message: message, cause: err}
}

// IsEngineError checks if an error is or wraps an engine error.
func IsEngineError(err error) bool {
	var engineErr Error
	return errors.As(err, &engineErr)
}

// RequestError represents an HTTP error with status code.
type RequestError struct {
	Message    string `json:"error"`  // User-facing error message
	StatusCode int    `json:"status"` // HTTP status code
	cause      error  `json:"-"`      // Underlying cause (not exposed in JSON)
}

// Error implements the error interface.
func (e RequestError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

// Unwrap implements the errors.Unwrap interface to support errors.Is and errors.As.
func (e RequestError) Unwrap() error {
	return e.cause
}

// WithCause attaches a cause to the request error for better debugging.
func (e RequestError) WithCause(cause error) RequestError {
	e.cause = cause
	return e
}

// NewRequestError creates a new request error with the given message and status code.
func NewRequestError(message string, statusCode int) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
	}
}

// Common request errors.
func NewNotFoundError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusNotFound,
	}
}

func NewBadRequestError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusBadRequest,
	}
}

func NewInternalServerError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusInternalServerError,
	}
}

func NewUnauthorizedError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusUnauthorized,
	}
}

func NewForbiddenError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusForbidden,
	}
}

// Helper error wrapping functions for better error handling

// IsRequestError checks if an error is or wraps a RequestError.
func IsRequestError(err error) bool {
	var reqErr RequestError
	return errors.As(err, &reqErr)
}

// IsNotFoundError checks if an error is or wraps a 404 RequestError.
func IsNotFoundError(err error) bool {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsBadRequestError checks if an error is or wraps a 400 RequestError.
func IsBadRequestError(err error) bool {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusBadRequest
	}
	return false
}

// IsInternalServerError checks if an error is or wraps a 500 RequestError.
func IsInternalServerError(err error) bool {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusInternalServerError
	}
	return false
}

// ErrorToStatusCode extracts the status code from an error or returns 500.
func ErrorToStatusCode(err error) int {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode
	}
	return http.StatusInternalServerError
}
