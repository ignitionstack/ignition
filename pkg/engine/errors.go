package engine

import (
	"fmt"
	"net/http"
)

// Common engine errors
var (
	ErrEngineNotInitialized = fmt.Errorf("engine is not initialized")
	ErrFunctionNotLoaded    = fmt.Errorf("function is not loaded")
	ErrFunctionNotFound     = fmt.Errorf("function not found")
)

type EngineError struct {
	message string
}

func (e EngineError) Error() string {
	return e.message
}

func NewEngineError(message string) error {
	return EngineError{message: message}
}

// IsEngineError checks if an error is an engine error
func IsEngineError(err error) bool {
	_, ok := err.(EngineError)
	return ok
}

type RequestError struct {
	Message    string `json:"error"`
	StatusCode int    `json:"status"`
	cause      error  `json:"-"`
}

func (e RequestError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

func (e RequestError) WithCause(cause error) RequestError {
	e.cause = cause
	return e
}

func NewRequestError(message string, statusCode int) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
	}
}

// Common request errors
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

func IsRequestError(err error) bool {
	_, ok := err.(RequestError)
	return ok
}

func IsNotFoundError(err error) bool {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode == http.StatusNotFound
	}
	return false
}

func IsBadRequestError(err error) bool {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode == http.StatusBadRequest
	}
	return false
}

func IsInternalServerError(err error) bool {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode == http.StatusInternalServerError
	}
	return false
}

func ErrorToStatusCode(err error) int {
	if reqErr, ok := err.(RequestError); ok {
		return reqErr.StatusCode
	}
	return http.StatusInternalServerError
}
