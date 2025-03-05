package engine

import (
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/config"
)

type Options struct {
	// Default timeout for function operations
	DefaultTimeout time.Duration

	// Capacity of the log store
	LogStoreCapacity int

	CircuitBreakerSettings components.CircuitBreakerSettings
	PluginManagerSettings  components.PluginManagerSettings
}

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

func OptionsFromConfig(cfg *config.Config) *Options {
	return &Options{
		DefaultTimeout:   cfg.Engine.DefaultTimeout,
		LogStoreCapacity: cfg.Engine.LogStoreCapacity,
		CircuitBreakerSettings: components.CircuitBreakerSettings{
			FailureThreshold: cfg.Engine.CircuitBreaker.FailureThreshold,
			ResetTimeout:     cfg.Engine.CircuitBreaker.ResetTimeout,
		},
		PluginManagerSettings: components.PluginManagerSettings{
			TTL:             cfg.Engine.PluginManager.TTL,
			CleanupInterval: cfg.Engine.PluginManager.CleanupInterval,
		},
	}
}

func (o *Options) WithDefaultTimeout(timeout time.Duration) *Options {
	o.DefaultTimeout = timeout
	return o
}

func (o *Options) WithLogStoreCapacity(capacity int) *Options {
	o.LogStoreCapacity = capacity
	return o
}

func (o *Options) WithCircuitBreakerSettings(settings components.CircuitBreakerSettings) *Options {
	o.CircuitBreakerSettings = settings
	return o
}

func (o *Options) WithPluginManagerSettings(settings components.PluginManagerSettings) *Options {
	o.PluginManagerSettings = settings
	return o
}
