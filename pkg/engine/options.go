package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
)

// EngineOptions defines configurable options for the engine
type EngineOptions struct {
	// Default timeout for function operations
	DefaultTimeout time.Duration

	// Capacity of the log store
	LogStoreCapacity int

	// Circuit breaker settings
	CircuitBreakerSettings components.CircuitBreakerSettings

	// Plugin manager settings
	PluginManagerSettings components.PluginManagerSettings
}

// DefaultEngineOptions returns a new EngineOptions with default values
func DefaultEngineOptions() *EngineOptions {
	return &EngineOptions{
		DefaultTimeout:   30 * time.Second,
		LogStoreCapacity: 1000,
		CircuitBreakerSettings: components.CircuitBreakerSettings{
			FailureThreshold: 5,
			ResetTimeout:     30 * time.Second,
		},
		PluginManagerSettings: components.PluginManagerSettings{
			TTL:             10 * time.Minute,
			CleanupInterval: 1 * time.Minute,
		},
	}
}

// WithDefaultTimeout sets the default timeout
func (o *EngineOptions) WithDefaultTimeout(timeout time.Duration) *EngineOptions {
	o.DefaultTimeout = timeout
	return o
}

// WithLogStoreCapacity sets the log store capacity
func (o *EngineOptions) WithLogStoreCapacity(capacity int) *EngineOptions {
	o.LogStoreCapacity = capacity
	return o
}

// WithCircuitBreakerSettings sets the circuit breaker settings
func (o *EngineOptions) WithCircuitBreakerSettings(settings components.CircuitBreakerSettings) *EngineOptions {
	o.CircuitBreakerSettings = settings
	return o
}

// WithPluginManagerSettings sets the plugin manager settings
func (o *EngineOptions) WithPluginManagerSettings(settings components.PluginManagerSettings) *EngineOptions {
	o.PluginManagerSettings = settings
	return o
}
