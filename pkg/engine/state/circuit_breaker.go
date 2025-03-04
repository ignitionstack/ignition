package state

import (
	"sync"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
)

// DefaultCircuitBreaker implements interfaces.CircuitBreaker
type DefaultCircuitBreaker struct {
	mu                  sync.RWMutex
	consecutiveFailures int
	lastFailure         time.Time
	isOpen              bool
	threshold           int
	resetTimeout        time.Duration
}

// NewCircuitBreaker creates a new DefaultCircuitBreaker
func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *DefaultCircuitBreaker {
	return &DefaultCircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// IsOpen implements interfaces.CircuitBreaker
func (cb *DefaultCircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// If it's open, check if it's time to reset
	if cb.isOpen && time.Since(cb.lastFailure) > cb.resetTimeout {
		// Note: We can't modify state here since we have a read lock
		// The real check will happen during the next failure/success
		return false
	}

	return cb.isOpen
}

// RecordSuccess implements interfaces.CircuitBreaker
func (cb *DefaultCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Reset consecutive failures
	cb.consecutiveFailures = 0

	// If the circuit was open and the reset timeout has passed, close it
	if cb.isOpen && time.Since(cb.lastFailure) > cb.resetTimeout {
		cb.isOpen = false
	}
}

// RecordFailure implements interfaces.CircuitBreaker
func (cb *DefaultCircuitBreaker) RecordFailure() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Record failure time
	cb.lastFailure = time.Now()

	// If the circuit is already open, check if it should be auto-reset
	if cb.isOpen {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.isOpen = false
			cb.consecutiveFailures = 1
			return false
		}
		return true
	}

	// Increment failure count
	cb.consecutiveFailures++

	// Check if threshold is reached
	if cb.consecutiveFailures >= cb.threshold {
		cb.isOpen = true
		return true
	}

	return false
}

// Reset implements interfaces.CircuitBreaker
func (cb *DefaultCircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.isOpen = false
	cb.consecutiveFailures = 0
}

// CircuitBreakerManagerImpl implements interfaces.CircuitBreakerManager
type CircuitBreakerManagerImpl struct {
	mu           sync.RWMutex
	breakers     map[string]*DefaultCircuitBreaker
	keyHandler   interfaces.KeyHandler
	threshold    int
	resetTimeout time.Duration
}

// NewCircuitBreakerManager creates a new CircuitBreakerManagerImpl
func NewCircuitBreakerManager(keyHandler interfaces.KeyHandler, threshold int, resetTimeout time.Duration) *CircuitBreakerManagerImpl {
	return &CircuitBreakerManagerImpl{
		breakers:     make(map[string]*DefaultCircuitBreaker),
		keyHandler:   keyHandler,
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// GetCircuitBreaker implements interfaces.CircuitBreakerManager
func (m *CircuitBreakerManagerImpl) GetCircuitBreaker(namespace, name string) interfaces.CircuitBreaker {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	breaker, exists := m.breakers[key]
	m.mu.RUnlock()

	if exists {
		return breaker
	}

	// Create a new circuit breaker
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check if another goroutine created the breaker
	if breaker, exists = m.breakers[key]; exists {
		return breaker
	}

	breaker = NewCircuitBreaker(m.threshold, m.resetTimeout)
	m.breakers[key] = breaker

	return breaker
}

// RecordSuccess implements interfaces.CircuitBreakerManager
func (m *CircuitBreakerManagerImpl) RecordSuccess(namespace, name string) {
	breaker := m.GetCircuitBreaker(namespace, name)
	breaker.RecordSuccess()
}

// RecordFailure implements interfaces.CircuitBreakerManager
func (m *CircuitBreakerManagerImpl) RecordFailure(namespace, name string) bool {
	breaker := m.GetCircuitBreaker(namespace, name)
	return breaker.RecordFailure()
}

// Reset implements interfaces.CircuitBreakerManager
func (m *CircuitBreakerManagerImpl) Reset(namespace, name string) {
	breaker := m.GetCircuitBreaker(namespace, name)
	breaker.Reset()
}

// RemoveCircuitBreaker removes a circuit breaker
func (m *CircuitBreakerManagerImpl) RemoveCircuitBreaker(namespace, name string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.breakers, key)
}
