package components

import (
	"sync"
	"sync/atomic"
	"time"
)

// Circuit breaker states as constants
const (
	stateClosed   = 0
	stateHalfOpen = 1
	stateOpen     = 2
)

// CircuitBreaker provides fault tolerance by temporarily disabling operations
// that repeatedly fail, allowing systems to fail fast rather than repeatedly
// retry failing operations.
type CircuitBreaker struct {
	failures         int32
	lastFailure      atomic.Value
	state            int32
	failureThreshold int32
	resetTimeout     time.Duration
	mutex            sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with default settings.
func NewCircuitBreaker() *CircuitBreaker {
	cb := &CircuitBreaker{
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
	cb.lastFailure.Store(time.Now())
	atomic.StoreInt32(&cb.state, stateClosed)
	atomic.StoreInt32(&cb.failures, 0)
	return cb
}

// NewCircuitBreakerWithOptions creates a new circuit breaker with custom settings.
func NewCircuitBreakerWithOptions(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{
		failureThreshold: int32(failureThreshold),
		resetTimeout:     resetTimeout,
	}
	cb.lastFailure.Store(time.Now())
	atomic.StoreInt32(&cb.state, stateClosed)
	atomic.StoreInt32(&cb.failures, 0)
	return cb
}

// RecordSuccess records a successful operation and resets the failure count if in half-open state.
func (cb *CircuitBreaker) RecordSuccess() {
	if atomic.LoadInt32(&cb.state) == stateHalfOpen {
		cb.mutex.Lock()
		defer cb.mutex.Unlock()

		if atomic.LoadInt32(&cb.state) == stateHalfOpen {
			atomic.StoreInt32(&cb.failures, 0)
			atomic.StoreInt32(&cb.state, stateClosed)
		}
	}
}

// RecordFailure records a failed operation and potentially opens the circuit.
// Returns true if the circuit is now open.
func (cb *CircuitBreaker) RecordFailure() bool {
	newCount := atomic.AddInt32(&cb.failures, 1)
	cb.lastFailure.Store(time.Now())

	if atomic.LoadInt32(&cb.state) == stateClosed && newCount >= cb.failureThreshold {
		cb.mutex.Lock()
		defer cb.mutex.Unlock()

		if atomic.LoadInt32(&cb.state) == stateClosed {
			atomic.StoreInt32(&cb.state, stateOpen)
		}
	}

	return atomic.LoadInt32(&cb.state) == stateOpen
}

// IsOpen checks if the circuit is currently open.
// If the reset timeout has expired, transitions to half-open state.
func (cb *CircuitBreaker) IsOpen() bool {
	if atomic.LoadInt32(&cb.state) != stateOpen {
		return false
	}

	lastFailureTime := cb.lastFailure.Load().(time.Time)
	if time.Since(lastFailureTime) > cb.resetTimeout {
		cb.mutex.Lock()
		defer cb.mutex.Unlock()

		if atomic.LoadInt32(&cb.state) == stateOpen &&
			time.Since(cb.lastFailure.Load().(time.Time)) > cb.resetTimeout {
			atomic.StoreInt32(&cb.state, stateHalfOpen)
			return false
		}
	}

	return atomic.LoadInt32(&cb.state) == stateOpen
}

// Reset resets the circuit breaker to its initial closed state.
func (cb *CircuitBreaker) Reset() {
	atomic.StoreInt32(&cb.failures, 0)
	atomic.StoreInt32(&cb.state, stateClosed)
}

// GetState returns the current state of the circuit breaker.
func (cb *CircuitBreaker) GetState() string {
	switch atomic.LoadInt32(&cb.state) {
	case stateClosed:
		return "closed"
	case stateHalfOpen:
		return "half-open"
	case stateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// GetFailureCount returns the current failure count.
func (cb *CircuitBreaker) GetFailureCount() int {
	return int(atomic.LoadInt32(&cb.failures))
}
