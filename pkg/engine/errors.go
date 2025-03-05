package engine

import (
	"errors"
	"fmt"
	"net/http"

	domainerrors "github.com/ignitionstack/ignition/pkg/engine/errors"
)

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

// NewBadRequestError is returned for invalid requests with the given message.
func NewBadRequestError(message string) error {
	return NewRequestError(message, http.StatusBadRequest)
}

func IsNotFoundError(err error) bool {
	var reqErr RequestError
	if errors.As(err, &reqErr) {
		return reqErr.StatusCode == http.StatusNotFound
	}
	return false
}

func NewNotFoundError(message string) error {
	return NewRequestError(message, http.StatusNotFound)
}

func NewInternalServerError(message string, err ...error) error {
	var actualErr error
	if len(err) > 0 {
		actualErr = err[0]
	}
	if message == "" {
		message = "Internal server error"
	}
	return NewRequestErrorWithCause(message, http.StatusInternalServerError, actualErr)
}

// EngineError base errors
var (
	ErrFunctionNotLoaded    = errors.New("function not loaded")
	ErrEngineNotInitialized = errors.New("engine not initialized")
)

func WrapEngineError(message string, err error) error {
	if err == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, err)
}

func isDomainError(err error) bool {
	var de *domainerrors.DomainError
	return errors.As(err, &de)
}

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

func DomainErrorToRequestError(err *domainerrors.DomainError) RequestError {
	return RequestError{
		Message:    err.Error(),
		StatusCode: DomainErrorToStatusCode(err),
		cause:      err,
	}
}
