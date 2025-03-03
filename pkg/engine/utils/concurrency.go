package utils

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/errors"
)

// Result represents a generic result with error.
type Result[T any] struct {
	Value T
	Err   error
}

// If the timeout is reached, a timeout error is returned.
func ExecuteWithTimeout[T any](timeout time.Duration, operation func() (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return ExecuteWithContext(ctx, operation)
}

// If the context is cancelled, a cancellation error is returned.
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

// WithBackpressure executes an operation with a semaphore to limit concurrency.
func WithBackpressure[T any](ctx context.Context, semaphore chan struct{}, operation func() (T, error)) (T, error) {
	var zero T

	// Try to acquire the semaphore with context timeout
	select {
	case semaphore <- struct{}{}:
		// Acquired the semaphore
		defer func() { <-semaphore }()

		// Execute the operation
		return operation()

	case <-ctx.Done():
		// Context cancelled while waiting for semaphore
		if ctx.Err() == context.DeadlineExceeded {
			return zero, errors.Wrap(errors.DomainExecution, errors.CodeExecutionTimeout,
				"Timed out waiting for execution slot", ctx.Err())
		}
		return zero, errors.Wrap(errors.DomainExecution, errors.CodeExecutionCancelled,
			"Operation was cancelled while waiting for execution slot", ctx.Err())
	}
}

// NewLimiter creates a semaphore channel for limiting concurrent operations.
func NewLimiter(maxConcurrent int) chan struct{} {
	if maxConcurrent <= 0 {
		maxConcurrent = 100 // Default to a reasonable limit
	}
	return make(chan struct{}, maxConcurrent)
}
