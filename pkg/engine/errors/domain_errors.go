package errors

import (
	"errors"
	"fmt"
)

// Domain enumerates the possible error domains
type Domain string

const (
	DomainEngine    Domain = "engine"
	DomainFunction  Domain = "function"
	DomainRegistry  Domain = "registry"
	DomainPlugin    Domain = "plugin"
	DomainExecution Domain = "execution"
)

// Code enumerates possible error codes for each domain
type Code string

// Engine error codes
const (
	CodeNotInitialized Code = "not_initialized"
	CodeInvalidState   Code = "invalid_state"
	CodeAlreadyRunning Code = "already_running"
	CodeShutdown       Code = "shutdown"
	CodeNotImplemented Code = "not_implemented"
	CodeInternalError  Code = "internal_error"
)

// Function error codes
const (
	CodeFunctionNotFound      Code = "function_not_found"
	CodeFunctionNotLoaded     Code = "function_not_loaded"
	CodeFunctionAlreadyLoaded Code = "function_already_loaded"
	CodeFunctionStopped       Code = "function_stopped"
	CodeInvalidFunction       Code = "invalid_function"
	CodeInvalidConfig         Code = "invalid_config"
)

// Registry error codes
const (
	CodeRegistryNotFound Code = "registry_not_found"
	CodeVersionNotFound  Code = "version_not_found"
	CodeTagNotFound      Code = "tag_not_found"
	CodeRegistryError    Code = "registry_error"
)

// Plugin error codes
const (
	CodePluginCreationFailed Code = "plugin_creation_failed"
	CodePluginNotFound       Code = "plugin_not_found"
	CodePluginAlreadyExists  Code = "plugin_already_exists"
)

// Execution error codes
const (
	CodeExecutionTimeout   Code = "execution_timeout"
	CodeExecutionCancelled Code = "execution_cancelled"
	CodeCircuitBreakerOpen Code = "circuit_breaker_open"
	CodeExecutionFailed    Code = "execution_failed"
)

// DomainError represents a domain-specific error.
type DomainError struct {
	// The error domain (engine, function, registry, etc.)
	ErrDomain Domain

	// Error code unique within the domain
	ErrCode Code

	// Human-readable error message
	Message string

	// Optional fields for context
	Namespace string
	Name      string
	Details   map[string]interface{}

	// Original error that caused this one, if any
	Cause error
}

// Error returns the error message.
func (e *DomainError) Error() string {
	// Basic message including domain and code
	msg := fmt.Sprintf("[%s:%s] %s", e.ErrDomain, e.ErrCode, e.Message)

	// Add function details if available
	if e.Namespace != "" && e.Name != "" {
		msg = fmt.Sprintf("%s (function: %s/%s)", msg, e.Namespace, e.Name)
	}

	// Add cause if available
	if e.Cause != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Cause)
	}

	return msg
}

// Unwrap returns the cause of this error
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// New creates a new DomainError.
func New(domain Domain, code Code, message string) *DomainError {
	return &DomainError{
		ErrDomain: domain,
		ErrCode:   code,
		Message:    message,
	}
}

// WithNamespace adds namespace context to the error
func (e *DomainError) WithNamespace(namespace string) *DomainError {
	e.Namespace = namespace
	return e
}

// WithName adds function name context to the error
func (e *DomainError) WithName(name string) *DomainError {
	e.Name = name
	return e
}

// WithCause adds the causing error
func (e *DomainError) WithCause(cause error) *DomainError {
	e.Cause = cause
	return e
}

// WithDetails adds additional context details
func (e *DomainError) WithDetails(details map[string]interface{}) *DomainError {
	e.Details = details
	return e
}

// Wrap wraps an error with domain context.
func Wrap(domain Domain, code Code, message string, err error) *DomainError {
	return &DomainError{
		ErrDomain: domain,
		ErrCode:   code,
		Message:    message,
		Cause:      err,
	}
}

// Is checks if an error is a DomainError with the specified domain and code.
func Is(err error, domain Domain, code Code) bool {
	var de *DomainError
	if errors.As(err, &de) {
		return de.ErrDomain == domain && de.ErrCode == code
	}
	return false
}

// Common engine errors
var (
	ErrEngineNotInitialized = New(DomainEngine, CodeNotInitialized, "Engine not initialized")
	ErrInvalidEngineState   = New(DomainEngine, CodeInvalidState, "Invalid engine state")
	ErrInternalError        = New(DomainEngine, CodeInternalError, "Internal engine error")
)

// Common function errors
var (
	ErrFunctionNotFound  = New(DomainFunction, CodeFunctionNotFound, "Function not found")
	ErrFunctionNotLoaded = New(DomainFunction, CodeFunctionNotLoaded, "Function not loaded")
	ErrFunctionStopped   = New(DomainFunction, CodeFunctionStopped, "Function is stopped")
)

// Common execution errors
var (
	ErrExecutionTimeout   = New(DomainExecution, CodeExecutionTimeout, "Function execution timed out")
	ErrCircuitBreakerOpen = New(DomainExecution, CodeCircuitBreakerOpen, "Circuit breaker is open")
)
