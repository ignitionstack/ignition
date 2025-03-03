package components

import (
	"sync"
	"time"
)

// CircuitBreakerManager manages multiple circuit breakers with concurrent access
type CircuitBreakerManager struct {
	// Using sync.Map for better concurrent access patterns
	circuitBreakers sync.Map

	// Default settings for new circuit breakers
	defaultSettings struct {
		failureThreshold int32
		resetTimeout     time.Duration
	}
}

// NewCircuitBreakerManager creates a new circuit breaker manager with default settings
func NewCircuitBreakerManager() *CircuitBreakerManager {
	manager := &CircuitBreakerManager{}
	manager.defaultSettings.failureThreshold = 5
	manager.defaultSettings.resetTimeout = 30 * time.Second

	return manager
}

// NewCircuitBreakerManagerWithOptions creates a new circuit breaker manager with custom settings
func NewCircuitBreakerManagerWithOptions(failureThreshold int, resetTimeout time.Duration) *CircuitBreakerManager {
	manager := &CircuitBreakerManager{}
	manager.defaultSettings.failureThreshold = int32(failureThreshold)
	manager.defaultSettings.resetTimeout = resetTimeout

	return manager
}

// GetCircuitBreaker retrieves a circuit breaker by key, creating it if it doesn't exist
func (cbm *CircuitBreakerManager) GetCircuitBreaker(key string) *CircuitBreaker {
	// Try to get existing circuit breaker
	if cb, exists := cbm.circuitBreakers.Load(key); exists {
		return cb.(*CircuitBreaker)
	}

	// Create a new circuit breaker with default settings
	newCB := NewCircuitBreakerWithOptions(
		int(cbm.defaultSettings.failureThreshold),
		cbm.defaultSettings.resetTimeout,
	)

	// Try to store it (may fail if another goroutine created one concurrently)
	actualCB, _ := cbm.circuitBreakers.LoadOrStore(key, newCB)
	return actualCB.(*CircuitBreaker)
}

// RemoveCircuitBreaker removes a circuit breaker from the manager
func (cbm *CircuitBreakerManager) RemoveCircuitBreaker(key string) {
	cbm.circuitBreakers.Delete(key)
}

// Reset resets all circuit breakers to their initial state
func (cbm *CircuitBreakerManager) Reset() {
	// Iterate through all circuit breakers and reset them
	cbm.circuitBreakers.Range(func(_, value interface{}) bool {
		cb := value.(*CircuitBreaker)
		cb.Reset()
		return true // continue iteration
	})
}

// GetCircuitBreakerState gets the state of a specific circuit breaker
func (cbm *CircuitBreakerManager) GetCircuitBreakerState(key string) string {
	if cb, exists := cbm.circuitBreakers.Load(key); exists {
		return cb.(*CircuitBreaker).GetState()
	}
	return ""
}

// GetAllCircuitBreakers returns all circuit breakers as a map
// Note: This creates a snapshot, not a live view
func (cbm *CircuitBreakerManager) GetAllCircuitBreakers() map[string]*CircuitBreaker {
	result := make(map[string]*CircuitBreaker)

	cbm.circuitBreakers.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(*CircuitBreaker)
		return true
	})

	return result
}
