package components

import (
	"sync"
	"time"
)

// CircuitBreaker provides fault tolerance by temporarily disabling operations
// that repeatedly fail, allowing systems to fail fast rather than repeatedly
// retry failing operations.
type CircuitBreaker struct {
	failures         int
	lastFailure      time.Time
	state            string // "closed", "open", or "half-open"
	failureThreshold int
	resetTimeout     time.Duration
	mutex            sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with default settings.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		failures:         0,
		state:            "closed",
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

// NewCircuitBreakerWithOptions creates a new circuit breaker with custom settings.
func NewCircuitBreakerWithOptions(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failures:         0,
		state:            "closed",
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
	}
}

// RecordSuccess records a successful operation and resets the failure count if in half-open state.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == "half-open" {
		cb.failures = 0
		cb.state = "closed"
	}
}

// RecordFailure records a failed operation and potentially opens the circuit.
// Returns true if the circuit is now open.
func (cb *CircuitBreaker) RecordFailure() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == "closed" && cb.failures >= cb.failureThreshold {
		cb.state = "open"
	}

	return cb.state == "open"
}

// IsOpen checks if the circuit is currently open.
// If the reset timeout has expired, transitions to half-open state.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if cb.state == "open" {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.state = "half-open"
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return false
		}
		return true
	}

	return false
}

// Reset resets the circuit breaker to its initial closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = "closed"
}

// GetState returns the current state of the circuit breaker.
func (cb *CircuitBreaker) GetState() string {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.state
}

// GetFailureCount returns the current failure count.
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.failures
}
