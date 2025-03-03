package components

import (
	"sync"
	"time"
)

// CircuitBreakerManager manages circuit breakers for functions.
type CircuitBreakerManager interface {
	// Get a circuit breaker for a function
	GetCircuitBreaker(key string) CircuitBreaker

	// Remove a circuit breaker
	RemoveCircuitBreaker(key string)

	// Reset all circuit breakers
	Reset()

	// Get state of a circuit breaker
	GetCircuitBreakerState(key string) string

	// Get all circuit breakers as a map
	GetAllCircuitBreakers() map[string]CircuitBreaker
}

// CircuitBreakerSettings holds configuration for circuit breakers.
type CircuitBreakerSettings struct {
	FailureThreshold int
	ResetTimeout     time.Duration
}

// defaultCircuitBreakerManager implements the CircuitBreakerManager interface.
type defaultCircuitBreakerManager struct {
	// Using sync.Map for better concurrent access patterns
	circuitBreakers sync.Map

	// Default settings for new circuit breakers
	failureThreshold int
	resetTimeout     time.Duration
}

// NewCircuitBreakerManager creates a new circuit breaker manager with default settings.
func NewCircuitBreakerManager() CircuitBreakerManager {
	return &defaultCircuitBreakerManager{
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

// NewCircuitBreakerManagerWithOptions creates a new circuit breaker manager with custom settings.
func NewCircuitBreakerManagerWithOptions(settings CircuitBreakerSettings) CircuitBreakerManager {
	return &defaultCircuitBreakerManager{
		failureThreshold: settings.FailureThreshold,
		resetTimeout:     settings.ResetTimeout,
	}
}

// GetCircuitBreaker retrieves a circuit breaker by key, creating it if it doesn't exist.
func (cbm *defaultCircuitBreakerManager) GetCircuitBreaker(key string) CircuitBreaker {
	// Try to get existing circuit breaker
	if cb, exists := cbm.circuitBreakers.Load(key); exists {
		circuitBreaker, ok := cb.(CircuitBreaker)
		if !ok {
			// This should never happen, but let's be defensive
			panic("invalid type in circuit breaker map")
		}
		return circuitBreaker
	}

	// Create a new circuit breaker with default settings
	newCB := NewCircuitBreakerWithOptions(
		cbm.failureThreshold,
		cbm.resetTimeout,
	)

	// Try to store it (may fail if another goroutine created one concurrently)
	actualCB, _ := cbm.circuitBreakers.LoadOrStore(key, newCB)
	circuitBreaker, ok := actualCB.(CircuitBreaker)
	if !ok {
		// This should never happen, but let's be defensive
		panic("invalid type in circuit breaker map")
	}
	return circuitBreaker
}

// RemoveCircuitBreaker removes a circuit breaker from the manager.
func (cbm *defaultCircuitBreakerManager) RemoveCircuitBreaker(key string) {
	cbm.circuitBreakers.Delete(key)
}

// Reset resets all circuit breakers to their initial state.
func (cbm *defaultCircuitBreakerManager) Reset() {
	// Iterate through all circuit breakers and reset them
	cbm.circuitBreakers.Range(func(_, value interface{}) bool {
		cb, ok := value.(CircuitBreaker)
		if !ok {
			// This should never happen, but let's be defensive
			return true
		}
		cb.Reset()
		return true // continue iteration
	})
}

// GetCircuitBreakerState gets the state of a specific circuit breaker.
func (cbm *defaultCircuitBreakerManager) GetCircuitBreakerState(key string) string {
	if cb, exists := cbm.circuitBreakers.Load(key); exists {
		circuitBreaker, ok := cb.(CircuitBreaker)
		if !ok {
			// This should never happen, but let's be defensive
			return ""
		}
		return circuitBreaker.GetState()
	}
	return ""
}

// GetAllCircuitBreakers returns all circuit breakers as a map.
// This is used primarily for testing and monitoring.
func (cbm *defaultCircuitBreakerManager) GetAllCircuitBreakers() map[string]CircuitBreaker {
	result := make(map[string]CircuitBreaker)

	cbm.circuitBreakers.Range(func(key, value interface{}) bool {
		k, kOk := key.(string)
		v, vOk := value.(CircuitBreaker)
		if kOk && vOk {
			result[k] = v
		}
		return true
	})

	return result
}
