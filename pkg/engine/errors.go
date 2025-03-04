package engine

import (
	"errors"
	"fmt"
	"net/http"

	domainerrors "github.com/ignitionstack/ignition/pkg/engine/errors"
)

// Legacy error types - deprecated but maintained for backward compatibility
// Use domain errors from errors/domain_errors.go instead.
var (
	// ErrEngineNotInitialized - use domainerrors.ErrEngineNotInitialized instead
	ErrEngineNotInitialized = errors.New("engine is not initialized")
	// ErrFunctionNotLoaded - use domainerrors.ErrFunctionNotLoaded instead
	ErrFunctionNotLoaded = errors.New("function is not loaded")
	// ErrFunctionNotFound - use domainerrors.ErrFunctionNotFound instead
	ErrFunctionNotFound = errors.New("function not found")
)

// NewEngineError creates a new domain error with the DomainEngine domain.
// Deprecated: Use domainerrors.New() or predefined errors instead.
func NewEngineError(message string) error {
	return domainerrors.New(domainerrors.DomainEngine, domainerrors.CodeInternalError, message)
}

// WrapEngineError wraps an error with domain context.
// Deprecated: Use domainerrors.Wrap() instead.
func WrapEngineError(message string, err error) error {
	return domainerrors.Wrap(domainerrors.DomainEngine, domainerrors.CodeInternalError, message, err)
}

// IsEngineError checks if an error is from the engine domain.
// Deprecated: Use domainerrors.Is() instead.
func IsEngineError(err error) bool {
	return domainerrors.IsDomain(err, domainerrors.DomainEngine)
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
	// First check if it's a RequestError
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode
	}

	// Then check domain errors and map them to appropriate HTTP codes
	var domainErr *domainerrors.DomainError
	if errors.As(err, &domainErr) {
		return DomainErrorToStatusCode(domainErr)
	}

	return http.StatusInternalServerError
}

// DomainErrorToStatusCode maps domain errors to HTTP status codes.
func DomainErrorToStatusCode(err *domainerrors.DomainError) int {
	switch err.ErrDomain {
	case domainerrors.DomainFunction:
		switch err.ErrCode {
		case domainerrors.CodeFunctionNotFound:
			return http.StatusNotFound
		case domainerrors.CodeFunctionNotLoaded:
			return http.StatusServiceUnavailable
		case domainerrors.CodeFunctionStopped:
			return http.StatusServiceUnavailable
		case domainerrors.CodeInvalidFunction:
			return http.StatusBadRequest
		}
	case domainerrors.DomainExecution:
		switch err.ErrCode {
		case domainerrors.CodeExecutionTimeout:
			return http.StatusGatewayTimeout
		case domainerrors.CodeCircuitBreakerOpen:
			return http.StatusServiceUnavailable
		}
	case domainerrors.DomainRegistry:
		switch err.ErrCode {
		case domainerrors.CodeRegistryNotFound:
			return http.StatusNotFound
		case domainerrors.CodeVersionNotFound:
			return http.StatusNotFound
		case domainerrors.CodeTagNotFound:
			return http.StatusNotFound
		}
	}

	// Default to internal server error for unmatched codes
	return http.StatusInternalServerError
}

// DomainErrorToRequestError converts a domain error to a request error.
func DomainErrorToRequestError(err *domainerrors.DomainError) RequestError {
	return RequestError{
		Message:    err.Error(),
		StatusCode: DomainErrorToStatusCode(err),
		cause:      err,
	}
}
