package errors

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrEngineNotInitialized = errors.New("engine not initialized")
	ErrFunctionNotLoaded    = errors.New("function not loaded")
	ErrFunctionTimeout      = errors.New("function execution timed out")
	ErrCircuitBreakerOpen   = errors.New("circuit breaker is open")

	ErrFunctionNotFound = errors.New("function not found")
	ErrDigestMismatch   = errors.New("digest mismatch")
	ErrTagNotFound      = errors.New("tag not found")

	ErrInvalidManifest = errors.New("invalid function manifest")

	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrBuildFailed         = errors.New("function build failed")
)

// Common error prefixes to clean up.
var errorPrefixes = []string{
	"Build failed: ",
	"builder initialization failed: ",
	"build failed: ",
	"failed to build function: ",
	"hash calculation failed: ",
	// Add more prefixes to clean up as needed
}

func WithDetails(err error, details string) error {
	return fmt.Errorf("%s: %w", details, err)
}

// CleanErrorMessage removes common verbose prefixes from error messages.
// to produce more concise error messages for the user
func CleanErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Try to find and remove each prefix
	for _, prefix := range errorPrefixes {
		if strings.Contains(errMsg, prefix) {
			// Find the first occurrence of this prefix
			index := strings.Index(errMsg, prefix)
			if index >= 0 {
				// Replace this prefix with an empty string, keeping any text before and after
				errMsg = errMsg[:index] + errMsg[index+len(prefix):]
			}
		}
	}

	// Capitalize the first letter of the cleaned message
	if len(errMsg) > 0 {
		errMsg = strings.ToUpper(errMsg[:1]) + errMsg[1:]
	}

	return errMsg
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
