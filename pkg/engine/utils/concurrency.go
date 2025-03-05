package utils

import (
	"context"

	"github.com/ignitionstack/ignition/pkg/engine/errors"
)

// Result represents a generic result with error.
type Result[T any] struct {
	Value T
	Err   error
}

func ExecuteWithContext[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	var zero T

	// Create a buffered channel to avoid goroutine leaks
	resultCh := make(chan Result[T], 1)

	// Execute the operation in a goroutine
	go func() {
		value, err := operation()

		// Try to send the result, handling the case where the context is cancelled
		select {
		case resultCh <- Result[T]{Value: value, Err: err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context was cancelled, nothing to do
		}
	}()

	// Wait for the result or context cancellation
	select {
	case result := <-resultCh:
		return result.Value, result.Err

	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return zero, errors.ErrExecutionTimeout
		}
		return zero, errors.Wrap(errors.DomainExecution, errors.CodeExecutionCancelled,
			"Operation was cancelled", ctx.Err())
	}
}
