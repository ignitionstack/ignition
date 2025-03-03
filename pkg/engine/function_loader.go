package engine

import (
	"context"
	"fmt"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/registry"
)

// FunctionLoader is responsible for loading, unloading, and managing function state
type FunctionLoader struct {
	registry        registry.Registry
	pluginManager   *components.PluginManager
	circuitBreakers *components.CircuitBreakerManager
	logStore        *logging.FunctionLogStore
	logger          logging.Logger
}

// NewFunctionLoader creates a new function loader
func NewFunctionLoader(registry registry.Registry, pluginManager *components.PluginManager, 
	circuitBreakers *components.CircuitBreakerManager, logStore *logging.FunctionLogStore, 
	logger logging.Logger) *FunctionLoader {
	return &FunctionLoader{
		registry:        registry,
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakers,
		logStore:        logStore,
		logger:          logger,
	}
}

// LoadFunction loads a function with context
func (l *FunctionLoader) LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return l.LoadFunctionWithForce(ctx, namespace, name, identifier, config, false)
}

// LoadFunctionWithForce loads a function with context and force option
func (l *FunctionLoader) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	l.logger.Printf("Loading function: %s/%s (identifier: %s, force: %v)", namespace, name, identifier, force)
	functionKey := components.GetFunctionKey(namespace, name)
	
	// Check if the function is stopped - only allow loading if force is true
	if l.IsStopped(namespace, name) && !force {
		l.logger.Printf("Function %s/%s is stopped and cannot be loaded without force option", namespace, name)
		l.logStore.AddLog(functionKey, logging.LevelError, "Cannot load stopped function. Use 'ignition function run' to explicitly load it")
		return fmt.Errorf("function was explicitly stopped - use 'ignition function run' to load it")
	}
	
	// If force is true and function is stopped, clear the stopped status
	if force && l.IsStopped(namespace, name) {
		l.logger.Printf("Force loading stopped function %s/%s - clearing stopped status", namespace, name)
		l.logStore.AddLog(functionKey, logging.LevelInfo, "Force loading stopped function - clearing stopped status")
		l.pluginManager.ClearStoppedStatus(functionKey)
	}

	l.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	// Create a copy of the config map
	configCopy := make(map[string]string)
	for k, v := range config {
		configCopy[k] = v
	}

	// Fetch the WASM bytes from the registry
	loadStart := time.Now()
	wasmBytes, versionInfo, err := l.pullWithContext(ctx, namespace, name, identifier)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to fetch WASM file from registry: %v", err)
		l.logger.Errorf(errMsg)
		l.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return fmt.Errorf("failed to fetch WASM file from registry: %w", err)
	}

	l.logStore.AddLog(functionKey, logging.LevelInfo,
		fmt.Sprintf("Function pulled from registry (size: %d bytes, time: %v)",
			len(wasmBytes), time.Since(loadStart)))

	actualDigest := versionInfo.FullDigest

	// Check if already loaded with same digest and config
	alreadyLoaded := l.pluginManager.IsPluginLoaded(functionKey)

	if alreadyLoaded {
		digestChanged := l.pluginManager.HasDigestChanged(functionKey, actualDigest)
		configChanged := l.pluginManager.HasConfigChanged(functionKey, configCopy)

		if !digestChanged && !configChanged {
			l.logger.Printf("Function %s already loaded with same digest and config", functionKey)
			l.logStore.AddLog(functionKey, logging.LevelInfo, "Function already loaded with same digest and config")
			return nil
		}

		if digestChanged {
			oldDigest, _ := l.pluginManager.GetPluginDigest(functionKey)
			l.logger.Printf("Function %s digest changed from %s to %s, reloading",
				functionKey, oldDigest, actualDigest)
			l.logStore.AddLog(functionKey, logging.LevelInfo,
				fmt.Sprintf("Function digest changed from %s to %s, reloading",
					oldDigest, actualDigest))
		}

		if configChanged {
			l.logger.Printf("Function %s configuration changed, reloading", functionKey)
			l.logStore.AddLog(functionKey, logging.LevelInfo, "Function configuration changed, reloading")
		}

		// Remove the old plugin from the plugin manager
		l.pluginManager.RemovePlugin(functionKey)

		// Remove circuit breaker for this function
		l.circuitBreakers.RemoveCircuitBreaker(functionKey)
	}

	// Create a new plugin instance
	initStart := time.Now()
	plugin, err := l.createPluginWithContext(ctx, wasmBytes, versionInfo, configCopy)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to initialize plugin: %v", err)
		l.logger.Errorf(errMsg)
		l.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	l.logStore.AddLog(functionKey, logging.LevelInfo,
		fmt.Sprintf("Plugin initialized successfully (time: %v)", time.Since(initStart)))

	// Store the plugin in the plugin manager
	l.pluginManager.StorePlugin(functionKey, plugin, actualDigest, configCopy)

	successMsg := fmt.Sprintf("Function loaded successfully: %s", functionKey)
	l.logger.Printf(successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, successMsg)
	return nil
}

// UnloadFunction unloads a function
func (l *FunctionLoader) UnloadFunction(namespace, name string) error {
	l.logger.Printf("Unloading function: %s/%s", namespace, name)
	functionKey := components.GetFunctionKey(namespace, name)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Unloading function")

	// Check if the function is loaded
	if !l.pluginManager.IsPluginLoaded(functionKey) {
		notLoadedMsg := fmt.Sprintf("Function %s is not loaded, nothing to unload", functionKey)
		l.logger.Printf(notLoadedMsg)
		l.logStore.AddLog(functionKey, logging.LevelInfo, notLoadedMsg)
		return nil
	}

	unloadStart := time.Now()

	// Remove the plugin from the plugin manager
	l.pluginManager.RemovePlugin(functionKey)

	// Remove circuit breaker for this function
	l.circuitBreakers.RemoveCircuitBreaker(functionKey)

	successMsg := fmt.Sprintf("Function %s unloaded successfully (time: %v)", functionKey, time.Since(unloadStart))
	l.logger.Printf(successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, successMsg)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Function unloaded - this is the final log entry")

	return nil
}

// StopFunction stops a function (prevents auto-reload)
func (l *FunctionLoader) StopFunction(namespace, name string) error {
	l.logger.Printf("Stopping function: %s/%s", namespace, name)
	functionKey := components.GetFunctionKey(namespace, name)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Stopping function")

	// Check if the function is already stopped
	if l.pluginManager.IsFunctionStopped(functionKey) {
		alreadyStoppedMsg := fmt.Sprintf("Function %s is already stopped", functionKey)
		l.logger.Printf(alreadyStoppedMsg)
		l.logStore.AddLog(functionKey, logging.LevelInfo, alreadyStoppedMsg)
		return nil
	}

	stopStart := time.Now()

	// Stop the function using the plugin manager's StopFunction method
	l.pluginManager.StopFunction(functionKey)

	// Remove circuit breaker for this function
	l.circuitBreakers.RemoveCircuitBreaker(functionKey)

	successMsg := fmt.Sprintf("Function %s stopped successfully (time: %v)", functionKey, time.Since(stopStart))
	l.logger.Printf(successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, successMsg)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Function stopped - will not be automatically reloaded")

	return nil
}

// IsLoaded checks if a function is loaded
func (l *FunctionLoader) IsLoaded(namespace, name string) bool {
	functionKey := components.GetFunctionKey(namespace, name)
	return l.pluginManager.IsPluginLoaded(functionKey)
}

// WasPreviouslyLoaded checks if a function was previously loaded
func (l *FunctionLoader) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	functionKey := components.GetFunctionKey(namespace, name)
	return l.pluginManager.WasPreviouslyLoaded(functionKey)
}

// IsStopped checks if a function is stopped
func (l *FunctionLoader) IsStopped(namespace, name string) bool {
	functionKey := components.GetFunctionKey(namespace, name)
	return l.pluginManager.IsFunctionStopped(functionKey)
}

// Helper methods

// pullWithContext fetches a WASM module with context
func (l *FunctionLoader) pullWithContext(ctx context.Context, namespace, name, identifier string) ([]byte, *registry.VersionInfo, error) {
	// Create a channel for the result
	type pullResult struct {
		wasmBytes   []byte
		versionInfo *registry.VersionInfo
		err         error
	}

	// Use a channel with buffer size 1 to prevent goroutine leaks
	resultCh := make(chan pullResult, 1)

	// Pull in a separate goroutine to allow for cancellation
	go func() {
		wasmBytes, versionInfo, err := l.registry.Pull(namespace, name, identifier)
		select {
		case resultCh <- pullResult{wasmBytes, versionInfo, err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context was cancelled, but we need to send the result to avoid goroutine leak
			select {
			case resultCh <- pullResult{nil, nil, ctx.Err()}:
			default:
				// Channel is already closed or full, nothing to do
			}
		}
	}()

	// Wait for the result or context cancellation
	var result pullResult
	select {
	case result = <-resultCh:
		// Result received
	case <-ctx.Done():
		// Context cancelled, wait for the result to avoid goroutine leak
		result = <-resultCh
	}

	return result.wasmBytes, result.versionInfo, result.err
}

// createPluginWithContext creates a plugin with context
func (l *FunctionLoader) createPluginWithContext(ctx context.Context, wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (*extism.Plugin, error) {
	// Create plugin in a cancellable goroutine
	type pluginResult struct {
		plugin *extism.Plugin
		err    error
	}

	pluginCh := make(chan pluginResult, 1)

	go func() {
		plugin, err := components.CreatePlugin(wasmBytes, versionInfo, config)
		select {
		case pluginCh <- pluginResult{plugin, err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context was cancelled, but cleanup and send result to avoid goroutine leak
			if plugin != nil && err == nil {
				plugin.Close(context.Background())
			}
			select {
			case pluginCh <- pluginResult{nil, ctx.Err()}:
			default:
				// Channel is already closed or full, nothing to do
			}
		}
	}()

	// Wait for plugin creation or context cancellation
	var pluginRes pluginResult
	select {
	case pluginRes = <-pluginCh:
		// Result received
	case <-ctx.Done():
		// Context cancelled, wait for the result to avoid goroutine leak
		pluginRes = <-pluginCh
	}

	return pluginRes.plugin, pluginRes.err
}