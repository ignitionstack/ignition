package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
)

// Options defines configurable options for the engine.
type Options struct {
	// Default timeout for function operations
	DefaultTimeout time.Duration

	// Capacity of the log store
	LogStoreCapacity int

	// Circuit breaker settings
	CircuitBreakerSettings components.CircuitBreakerSettings

	// Plugin manager settings
	PluginManagerSettings components.PluginManagerSettings
}

// DefaultEngineOptions returns a new Options with default values.
func DefaultEngineOptions() *Options {
	return &Options{
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

// WithDefaultTimeout sets the default timeout.
func (o *Options) WithDefaultTimeout(timeout time.Duration) *Options {
	o.DefaultTimeout = timeout
	return o
}

// WithLogStoreCapacity sets the log store capacity.
func (o *Options) WithLogStoreCapacity(capacity int) *Options {
	o.LogStoreCapacity = capacity
	return o
}

// WithCircuitBreakerSettings sets the circuit breaker settings.
func (o *Options) WithCircuitBreakerSettings(settings components.CircuitBreakerSettings) *Options {
	o.CircuitBreakerSettings = settings
	return o
}

// WithPluginManagerSettings sets the plugin manager settings.
func (o *Options) WithPluginManagerSettings(settings components.PluginManagerSettings) *Options {
	o.PluginManagerSettings = settings
	return o
}
