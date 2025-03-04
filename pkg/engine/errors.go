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

// Error implements the error interface.
func (e RequestError) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause of the error.
func (e RequestError) Unwrap() error {
	return e.cause
}

// NewRequestError creates a new RequestError.
func NewRequestError(message string, statusCode int) RequestError {
	return RequestError{
		Message:    message,
		StatusCode: statusCode,
	}
}

// NewRequestErrorWithCause creates a new RequestError with a cause.
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

// DomainErrorToStatusCode maps domain errors to HTTP status codes.
func DomainErrorToStatusCode(err *domainerrors.DomainError) int {
	switch err.ErrDomain {
	case domainerrors.DomainEngine:
		switch err.ErrCode {
		case domainerrors.CodeNotInitialized:
			return http.StatusServiceUnavailable
		case domainerrors.CodeInvalidState:
			return http.StatusInternalServerError
		case domainerrors.CodeAlreadyRunning:
			return http.StatusConflict
		case domainerrors.CodeShutdown:
			return http.StatusServiceUnavailable
		case domainerrors.CodeNotImplemented:
			return http.StatusNotImplemented
		case domainerrors.CodeInternalError:
			return http.StatusInternalServerError
		default:
			return http.StatusInternalServerError
		}
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
		case domainerrors.CodeFunctionAlreadyLoaded:
			return http.StatusConflict
		case domainerrors.CodeInvalidConfig:
			return http.StatusBadRequest
		default:
			return http.StatusInternalServerError
		}
	case domainerrors.DomainExecution:
		switch err.ErrCode {
		case domainerrors.CodeExecutionTimeout:
			return http.StatusGatewayTimeout
		case domainerrors.CodeCircuitBreakerOpen:
			return http.StatusServiceUnavailable
		case domainerrors.CodeExecutionCancelled:
			return http.StatusRequestTimeout
		case domainerrors.CodeExecutionFailed:
			return http.StatusInternalServerError
		default:
			return http.StatusInternalServerError
		}
	case domainerrors.DomainRegistry:
		switch err.ErrCode {
		case domainerrors.CodeRegistryNotFound:
			return http.StatusNotFound
		case domainerrors.CodeVersionNotFound:
			return http.StatusNotFound
		case domainerrors.CodeTagNotFound:
			return http.StatusNotFound
		case domainerrors.CodeRegistryError:
			return http.StatusInternalServerError
		default:
			return http.StatusInternalServerError
		}
	case domainerrors.DomainPlugin:
		switch err.ErrCode {
		case domainerrors.CodePluginCreationFailed:
			return http.StatusInternalServerError
		case domainerrors.CodePluginNotFound:
			return http.StatusNotFound
		case domainerrors.CodePluginAlreadyExists:
			return http.StatusConflict
		default:
			return http.StatusInternalServerError
		}
	default:
		return http.StatusInternalServerError
	}
}

// DomainErrorToRequestError converts a domain error to a request error.
func DomainErrorToRequestError(err *domainerrors.DomainError) RequestError {
	return RequestError{
		Message:    err.Error(),
		StatusCode: DomainErrorToStatusCode(err),
		cause:      err,
	}
}
