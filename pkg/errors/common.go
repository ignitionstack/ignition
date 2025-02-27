package errors

import (
	"errors"
	"fmt"
)

var (
	ErrEngineNotInitialized = errors.New("engine not initialized")
	ErrFunctionNotLoaded    = errors.New("function not loaded")
	ErrFunctionTimeout      = errors.New("function execution timed out")
	ErrCircuitBreakerOpen   = errors.New("circuit breaker is open")
	
	ErrFunctionNotFound     = errors.New("function not found")
	ErrDigestMismatch       = errors.New("digest mismatch")
	ErrTagNotFound          = errors.New("tag not found")
	
	ErrInvalidManifest      = errors.New("invalid function manifest")
	
	ErrUnsupportedLanguage  = errors.New("unsupported language")
	ErrBuildFailed          = errors.New("function build failed")
)

func WithDetails(err error, details string) error {
	return fmt.Errorf("%s: %w", details, err)
}

func AsEngineError(err error, functionKey string, operation string) error {
	return fmt.Errorf("%s failed for function %s: %w", operation, functionKey, err)
}

func IsEngineNotInitialized(err error) bool {
	return errors.Is(err, ErrEngineNotInitialized)
}

func IsFunctionNotLoaded(err error) bool {
	return errors.Is(err, ErrFunctionNotLoaded)
}

func IsCircuitBreakerOpen(err error) bool {
	return errors.Is(err, ErrCircuitBreakerOpen)
}