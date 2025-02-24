package engine

import (
	"fmt"
	"net/http"
)

// EngineError represents a generic engine error
type EngineError struct {
	msg string
}

// Error returns the error message
func (e *EngineError) Error() string {
	return e.msg
}

// NewEngineError creates a new engine error
func NewEngineError(msg string) *EngineError {
	return &EngineError{msg: msg}
}

// Common engine errors
var (
	ErrFunctionNotLoaded    = NewEngineError("function not loaded")
	ErrInvalidConfig        = NewEngineError("invalid configuration")
	ErrPluginCreation       = NewEngineError("failed to create plugin")
	ErrEngineNotInitialized = NewEngineError("engine not initialized")
)

// RequestError represents an HTTP request error
type RequestError struct {
	Message    string
	StatusCode int
	Cause      error
}

// Error returns the error message
func (e RequestError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// WithCause sets the cause of the error
func (e RequestError) WithCause(err error) RequestError {
	e.Cause = err
	return e
}

// NewRequestError creates a new request error with a custom status code
func NewRequestError(message string, statusCode int) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
	}
}

// Common HTTP request errors
func NewBadRequestError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusBadRequest,
	}
}

func NewNotFoundError(message string) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: http.StatusNotFound,
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

// IsNotFoundError checks if an error is a not found error
func IsNotFoundError(err error) bool {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsBadRequestError checks if an error is a bad request error
func IsBadRequestError(err error) bool {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode == http.StatusBadRequest
	}
	return false
}
