package engine

import (
	"errors"
	"fmt"
	"net/http"

	domainerrors "github.com/ignitionstack/ignition/pkg/engine/errors"
)

// RequestError represents a client-facing HTTP error.
type RequestError struct {
	Message    string
	StatusCode int
	cause      error
}

func (e RequestError) Error() string {
	return e.Message
}

func (e RequestError) Unwrap() error {
	return e.cause
}

func NewRequestError(message string, statusCode int) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
	}
}

func NewRequestErrorWithCause(message string, statusCode int, cause error) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
		cause:      cause,
	}
}

// FunctionNotFoundError is returned when a function is not found.
func FunctionNotFoundError(functionKey string) error {
	message := fmt.Sprintf("Function not found: %s", functionKey)
	return NewRequestError(message, http.StatusNotFound)
}

// IsNotFoundError checks if an error is a 404 Not Found error.
func IsNotFoundError(err error) bool {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusNotFound
	}
	return false
}

// FunctionNotLoadedError is returned when a function is not loaded.
func FunctionNotLoadedError(functionKey string) error {
	message := fmt.Sprintf("Function not loaded: %s", functionKey)
	return NewRequestError(message, http.StatusServiceUnavailable)
}

// InvalidRequestError is returned for invalid requests.
func InvalidRequestError(message string) error {
	return NewRequestError(message, http.StatusBadRequest)
}

// NewBadRequestError is returned for invalid requests with the given message.
func NewBadRequestError(message string) error {
	return NewRequestError(message, http.StatusBadRequest)
}

// MethodNotAllowedError is returned for unsupported HTTP methods.
func MethodNotAllowedError(method string) error {
	message := fmt.Sprintf("Method not allowed: %s", method)
	return NewRequestError(message, http.StatusMethodNotAllowed)
}

// InternalServerError is returned for internal server errors.
func InternalServerError(message string, err error) error {
	if message == "" {
		message = "Internal server error"
	}
	return NewRequestErrorWithCause(message, http.StatusInternalServerError, err)
}

// NewInternalServerError is returned for internal server errors with the given message.
func NewInternalServerError(message string, err ...error) error {
	var actualErr error
	if len(err) > 0 {
		actualErr = err[0]
	}
	return InternalServerError(message, actualErr)
}

// NewNotFoundError is returned when a resource is not found.
func NewNotFoundError(message string) error {
	return NewRequestError(message, http.StatusNotFound)
}

// EngineError base errors
var (
	ErrFunctionNotFound     = errors.New("function not found")
	ErrFunctionNotLoaded    = errors.New("function not loaded")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrTimeout              = errors.New("execution timed out")
	ErrEngineNotInitialized = errors.New("engine not initialized")
)

// WrapEngineError wraps an error with context.
func WrapEngineError(message string, err error) error {
	if err == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, err)
}

// isDomainError checks if an error is a DomainError.
func isDomainError(err error) bool {
	var de *domainerrors.DomainError
	return errors.As(err, &de)
}

// errorCodeStatusMap maps domain and error codes to HTTP status codes
var errorCodeStatusMap = map[domainerrors.Domain]map[domainerrors.Code]int{
	domainerrors.DomainEngine: {
		domainerrors.CodeNotInitialized: http.StatusServiceUnavailable,
		domainerrors.CodeInvalidState:   http.StatusInternalServerError,
		domainerrors.CodeAlreadyRunning: http.StatusConflict,
		domainerrors.CodeShutdown:       http.StatusServiceUnavailable,
		domainerrors.CodeNotImplemented: http.StatusNotImplemented,
		domainerrors.CodeInternalError:  http.StatusInternalServerError,
	},
	domainerrors.DomainFunction: {
		domainerrors.CodeFunctionNotFound:      http.StatusNotFound,
		domainerrors.CodeFunctionNotLoaded:     http.StatusServiceUnavailable,
		domainerrors.CodeFunctionAlreadyLoaded: http.StatusConflict,
		domainerrors.CodeFunctionStopped:       http.StatusServiceUnavailable,
		domainerrors.CodeInvalidFunction:       http.StatusBadRequest,
		domainerrors.CodeInvalidConfig:         http.StatusBadRequest,
	},
	domainerrors.DomainExecution: {
		domainerrors.CodeExecutionTimeout:   http.StatusGatewayTimeout,
		domainerrors.CodeExecutionCancelled: http.StatusRequestTimeout,
		domainerrors.CodeCircuitBreakerOpen: http.StatusServiceUnavailable,
		domainerrors.CodeExecutionFailed:    http.StatusInternalServerError,
	},
	domainerrors.DomainRegistry: {
		domainerrors.CodeRegistryNotFound: http.StatusNotFound,
		domainerrors.CodeVersionNotFound:  http.StatusNotFound,
		domainerrors.CodeTagNotFound:      http.StatusNotFound,
		domainerrors.CodeRegistryError:    http.StatusInternalServerError,
	},
	domainerrors.DomainPlugin: {
		domainerrors.CodePluginCreationFailed: http.StatusInternalServerError,
		domainerrors.CodePluginNotFound:       http.StatusNotFound,
		domainerrors.CodePluginAlreadyExists:  http.StatusConflict,
	},
}

// defaultStatusCodes for each domain when a specific code mapping is not found
var defaultStatusCodes = map[domainerrors.Domain]int{
	domainerrors.DomainEngine:    http.StatusInternalServerError,
	domainerrors.DomainFunction:  http.StatusInternalServerError,
	domainerrors.DomainExecution: http.StatusInternalServerError,
	domainerrors.DomainRegistry:  http.StatusInternalServerError,
	domainerrors.DomainPlugin:    http.StatusInternalServerError,
}

// DomainErrorToStatusCode maps domain errors to HTTP status codes.
func DomainErrorToStatusCode(err *domainerrors.DomainError) int {
	// Get the map for this domain
	if codeMap, ok := errorCodeStatusMap[err.ErrDomain]; ok {
		// Look up the status code for this error code
		if statusCode, ok := codeMap[err.ErrCode]; ok {
			return statusCode
		}

		// If no specific mapping, use the default for this domain
		if defaultCode, ok := defaultStatusCodes[err.ErrDomain]; ok {
			return defaultCode
		}
	}

	// Fallback default if domain is not recognized
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
