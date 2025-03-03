package components

import (
	"sync"
	"time"
)

type CircuitBreakerManager struct {
	circuitBreakers map[string]*CircuitBreaker
	mutex           sync.RWMutex
	defaultSettings struct {
		failureThreshold int
		resetTimeout     time.Duration
	}
}

func NewCircuitBreakerManager() *CircuitBreakerManager {
	manager := &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
	manager.defaultSettings.failureThreshold = 5
	manager.defaultSettings.resetTimeout = 30 * time.Second

	return manager
}

func NewCircuitBreakerManagerWithOptions(failureThreshold int, resetTimeout time.Duration) *CircuitBreakerManager {
	manager := &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
	manager.defaultSettings.failureThreshold = failureThreshold
	manager.defaultSettings.resetTimeout = resetTimeout

	return manager
}

func (cbm *CircuitBreakerManager) GetCircuitBreaker(key string) *CircuitBreaker {
	cbm.mutex.RLock()
	cb, exists := cbm.circuitBreakers[key]
	cbm.mutex.RUnlock()

	if !exists {
		cbm.mutex.Lock()
		// Check again in case another goroutine created it while we were waiting for the lock
		cb, exists = cbm.circuitBreakers[key]
		if !exists {
			cb = NewCircuitBreakerWithOptions(
				cbm.defaultSettings.failureThreshold,
				cbm.defaultSettings.resetTimeout,
			)
			cbm.circuitBreakers[key] = cb
		}
		cbm.mutex.Unlock()
	}

	return cb
}

func (cbm *CircuitBreakerManager) RemoveCircuitBreaker(key string) {
	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	delete(cbm.circuitBreakers, key)
}

func (cbm *CircuitBreakerManager) Reset() {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	for _, cb := range cbm.circuitBreakers {
		cb.Reset()
	}
}

func (cbm *CircuitBreakerManager) GetCircuitBreakerState(key string) string {
	cbm.mutex.RLock()
	cb, exists := cbm.circuitBreakers[key]
	cbm.mutex.RUnlock()

	if !exists {
		return ""
	}

	return cb.GetState()
}
