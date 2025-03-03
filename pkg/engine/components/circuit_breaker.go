package components

import (
	"sync"
	"time"
)

// CircuitBreaker defines the interface for a circuit breaker
type CircuitBreaker interface {
	// Record a successful operation
	RecordSuccess()

	// Record a failed operation
	RecordFailure() bool

	// Check if the circuit is open
	IsOpen() bool

	// Reset the circuit breaker
	Reset()

	// Get the current state
	GetState() string

	// Get the current failure count
	GetFailureCount() int
}

// Circuit breaker states as string constants
const (
	StateClosed   = "closed"
	StateHalfOpen = "half-open"
	StateOpen     = "open"
)

// defaultCircuitBreaker provides an implementation of the CircuitBreaker interface
// that is simplified to use a consistent locking strategy
type defaultCircuitBreaker struct {
	// Mutex for protecting all state changes
	mutex sync.RWMutex

	// Current state
	state string

	// Number of consecutive failures
	failures int

	// Timestamp of the last failure
	lastFailure time.Time

	// Settings
	failureThreshold int
	resetTimeout     time.Duration
}

// NewCircuitBreaker creates a new circuit breaker with default settings
func NewCircuitBreaker() *defaultCircuitBreaker {
	return &defaultCircuitBreaker{
		state:            StateClosed,
		failures:         0,
		lastFailure:      time.Now(),
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

// NewCircuitBreakerWithOptions creates a new circuit breaker with custom settings
func NewCircuitBreakerWithOptions(failureThreshold int, resetTimeout time.Duration) *defaultCircuitBreaker {
	return &defaultCircuitBreaker{
		state:            StateClosed,
		failures:         0,
		lastFailure:      time.Now(),
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
	}
}

// RecordSuccess records a successful operation and resets the failure count if in half-open state
func (cb *defaultCircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == StateHalfOpen {
		cb.failures = 0
		cb.state = StateClosed
	}
}

// RecordFailure records a failed operation and potentially opens the circuit
// Returns true if the circuit is now open
func (cb *defaultCircuitBreaker) RecordFailure() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == StateClosed && cb.failures >= cb.failureThreshold {
		cb.state = StateOpen
	}

	return cb.state == StateOpen
}

// IsOpen checks if the circuit is currently open
// If the reset timeout has expired, transitions to half-open state
func (cb *defaultCircuitBreaker) IsOpen() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state != StateOpen {
		return false
	}

	// Check if reset timeout has expired
	if time.Since(cb.lastFailure) > cb.resetTimeout {
		cb.state = StateHalfOpen
		return false
	}

	return true
}

// Reset resets the circuit breaker to its initial closed state
func (cb *defaultCircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = StateClosed
}

// GetState returns the current state of the circuit breaker
func (cb *defaultCircuitBreaker) GetState() string {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *defaultCircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.failures
}
