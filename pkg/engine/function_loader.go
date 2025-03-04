package engine

import (
	"context"
	"fmt"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/engine/utils"
	"github.com/ignitionstack/ignition/pkg/registry"
)

type FunctionLoader struct {
	registry        registry.Registry
	pluginManager   PluginManager
	circuitBreakers CircuitBreakerManager
	logStore        *logging.FunctionLogStore
	logger          logging.Logger
}

func NewFunctionLoader(registry registry.Registry, pluginManager PluginManager,
	circuitBreakers CircuitBreakerManager, logStore *logging.FunctionLogStore,
	logger logging.Logger) *FunctionLoader {
	return &FunctionLoader{
		registry:        registry,
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakers,
		logStore:        logStore,
		logger:          logger,
	}
}

// LoadFunction loads a function with the specified identifier and configuration.
// This is a convenience method that calls LoadFunctionWithForce with force=false.
func (l *FunctionLoader) LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return l.LoadFunctionWithForce(ctx, namespace, name, identifier, config, false)
}

// LoadFunctionWithForce loads a function with the specified identifier and configuration,
// with the option to force loading even if the function is marked as stopped.
func (l *FunctionLoader) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	l.logger.Printf("Loading function: %s/%s (identifier: %s, force: %v)", namespace, name, identifier, force)
	functionKey := GetFunctionKey(namespace, name)

	// Validate loading permissions
	if err := l.validateLoadPermissions(namespace, name, force); err != nil {
		return err
	}

	l.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	// Create a deep copy of the config map to prevent side effects
	configCopy := l.copyConfig(config)

	// Fetch the WASM bytes from the registry
	loadStart := time.Now()
	wasmBytes, versionInfo, err := l.pullWithContext(ctx, namespace, name, identifier)
	if err != nil {
		return l.handlePullError(functionKey, err)
	}

	// Log success and record detailed information
	l.logStore.AddLog(functionKey, logging.LevelInfo,
		fmt.Sprintf("Function pulled from registry (size: %d bytes, time: %v)",
			len(wasmBytes), time.Since(loadStart)))

	actualDigest := versionInfo.FullDigest

	// Check if the function is already loaded and handle accordingly
	if err := l.handleExistingFunction(functionKey, configCopy, actualDigest); err != nil {
		return err
	}

	// Create and initialize the plugin
	return l.createAndStorePlugin(ctx, functionKey, wasmBytes, versionInfo, configCopy, actualDigest)
}

// validateLoadPermissions checks if a function can be loaded based on its stopped status.
// Returns nil if the function can be loaded, error otherwise.
func (l *FunctionLoader) validateLoadPermissions(namespace, name string, force bool) error {
	functionKey := GetFunctionKey(namespace, name)
	isStopped := l.IsStopped(namespace, name)

	// Function is not stopped or force is true - allow loading
	if !isStopped || (isStopped && force) {
		// If force-loading a stopped function, clear the stopped status
		if isStopped && force {
			l.logger.Printf("Force loading stopped function %s - clearing stopped status", functionKey)
			l.logStore.AddLog(functionKey, logging.LevelInfo, "Force loading stopped function - clearing stopped status")
			l.pluginManager.ClearStoppedStatus(functionKey)
		}
		return nil
	}

	// Function is stopped and force is false - prevent loading
	l.logger.Printf("Function %s is stopped and cannot be loaded without force option", functionKey)
	l.logStore.AddLog(functionKey, logging.LevelError,
		"Cannot load stopped function. Use 'ignition function run' to explicitly load it")
	return WrapEngineError("function was explicitly stopped - use 'ignition function run' to load it", nil)
}

// copyConfig creates a deep copy of a configuration map.
func (l *FunctionLoader) copyConfig(config map[string]string) map[string]string {
	configCopy := make(map[string]string, len(config))
	for k, v := range config {
		configCopy[k] = v
	}
	return configCopy
}

// logAndWrapError logs an error and wraps it with an EngineError.
// This centralizes error handling to ensure consistent logging and wrapping.
func (l *FunctionLoader) logAndWrapError(functionKey, operation string, err error) error {
	errMsg := fmt.Sprintf("%s: %v", operation, err)
	l.logger.Errorf(errMsg)
	l.logStore.AddLog(functionKey, logging.LevelError, errMsg)
	return WrapEngineError(operation, err)
}

// handlePullError logs and wraps errors from pulling a function from the registry.
func (l *FunctionLoader) handlePullError(functionKey string, err error) error {
	return l.logAndWrapError(functionKey, "failed to fetch WASM file from registry", err)
}

// handleExistingFunction checks if a function needs to be reloaded based on changes.
// Returns nil if no reload is needed or if reload preparation was successful.
func (l *FunctionLoader) handleExistingFunction(functionKey string, configCopy map[string]string, actualDigest string) error {
	// If function is not loaded, nothing to do
	if !l.pluginManager.IsPluginLoaded(functionKey) {
		return nil
	}

	// Check if anything has changed
	digestChanged := l.pluginManager.HasDigestChanged(functionKey, actualDigest)
	configChanged := l.pluginManager.HasConfigChanged(functionKey, configCopy)

	// If nothing changed, we can skip reloading
	if !digestChanged && !configChanged {
		l.logger.Printf("Function %s already loaded with same digest and config", functionKey)
		l.logStore.AddLog(functionKey, logging.LevelInfo, "Function already loaded with same digest and config")
		return nil
	}

	// Log what changed for debugging
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

	// Cleanup resources for reload
	l.pluginManager.RemovePlugin(functionKey)
	l.circuitBreakers.RemoveCircuitBreaker(functionKey)

	return nil
}

// createAndStorePlugin creates a new plugin instance and stores it in the plugin manager
//
//nolint:whitespace // Complex function signature with many parameters causes whitespace linting issues
func (l *FunctionLoader) createAndStorePlugin(
	ctx context.Context, key string, wasm []byte, vi *registry.VersionInfo,
	cfg map[string]string, dg string) error {

	// Create a new plugin instance
	initStart := time.Now()
	plugin, err := l.createPluginWithContext(ctx, wasm, vi, cfg)
	if err != nil {
		return l.logAndWrapError(key, "failed to initialize plugin", err)
	}

	// Log successful initialization
	l.logStore.AddLog(key, logging.LevelInfo,
		fmt.Sprintf("Plugin initialized successfully (time: %v)", time.Since(initStart)))

	// Store the plugin in the plugin manager
	l.pluginManager.StorePlugin(key, plugin, dg, cfg)

	// Log success
	successMsg := fmt.Sprintf("Function loaded successfully: %s", key)
	l.logger.Printf(successMsg)
	l.logStore.AddLog(key, logging.LevelInfo, successMsg)
	return nil
}

// UnloadFunction unloads a function, removing it from memory but preserving its
// configuration for potential future reloading.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - error: Any error that occurred during unloading
func (l *FunctionLoader) UnloadFunction(namespace, name string) error {
	l.logger.Printf("Unloading function: %s/%s", namespace, name)
	functionKey := GetFunctionKey(namespace, name)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Unloading function")

	// Check if the function is loaded
	if !l.pluginManager.IsPluginLoaded(functionKey) {
		notLoadedMsg := fmt.Sprintf("Function %s is not loaded, nothing to unload", functionKey)
		l.logger.Printf(notLoadedMsg)
		l.logStore.AddLog(functionKey, logging.LevelInfo, notLoadedMsg)
		return nil
	}

	// Track performance
	unloadStart := time.Now()

	// Perform the unload operation
	if err := l.performUnload(functionKey); err != nil {
		return err
	}

	// Log success
	successMsg := fmt.Sprintf("Function %s unloaded successfully (time: %v)", functionKey, time.Since(unloadStart))
	l.logger.Printf(successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, "Function unloaded - this is the final log entry")

	return nil
}

// performUnload does the actual work of unloading a function and handling errors.
func (l *FunctionLoader) performUnload(functionKey string) error {
	// Remove the plugin from the plugin manager
	if !l.pluginManager.RemovePlugin(functionKey) {
		// This should not normally happen since we already checked if it's loaded
		l.logger.Printf("Warning: Failed to remove plugin %s, it may have been removed concurrently", functionKey)
		l.logStore.AddLog(functionKey, logging.LevelWarning, "Failed to remove plugin, it may have been removed concurrently")
	}

	// Remove circuit breaker for this function
	l.circuitBreakers.RemoveCircuitBreaker(functionKey)

	return nil
}

// StopFunction stops a function and marks it as explicitly stopped, which prevents
// it from being automatically reloaded. It can only be reloaded with the force option.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - error: Any error that occurred during stopping
func (l *FunctionLoader) StopFunction(namespace, name string) error {
	l.logger.Printf("Stopping function: %s/%s", namespace, name)
	functionKey := GetFunctionKey(namespace, name)

	l.logStore.AddLog(functionKey, logging.LevelInfo, "Stopping function")

	// Check if the function is already stopped
	if l.pluginManager.IsFunctionStopped(functionKey) {
		alreadyStoppedMsg := fmt.Sprintf("Function %s is already stopped", functionKey)
		l.logger.Printf(alreadyStoppedMsg)
		l.logStore.AddLog(functionKey, logging.LevelInfo, alreadyStoppedMsg)
		return nil
	}

	// Track performance
	stopStart := time.Now()

	// Perform the stop operation
	if err := l.performStop(functionKey); err != nil {
		return err
	}

	// Log success
	successMsg := fmt.Sprintf("Function %s stopped successfully (time: %v)", functionKey, time.Since(stopStart))
	l.logger.Printf(successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, successMsg)
	l.logStore.AddLog(functionKey, logging.LevelInfo, "Function stopped - will not be automatically reloaded")

	return nil
}

// performStop does the actual work of stopping a function and handling errors.
func (l *FunctionLoader) performStop(functionKey string) error {
	// Stop the function using the plugin manager's StopFunction method
	if !l.pluginManager.StopFunction(functionKey) {
		// This is not an error - it just means the function wasn't loaded to begin with
		l.logger.Printf("Function %s was not loaded when stopping it", functionKey)
		l.logStore.AddLog(functionKey, logging.LevelInfo, "Function was not loaded when stopping it")
	}

	// Remove circuit breaker for this function
	l.circuitBreakers.RemoveCircuitBreaker(functionKey)

	return nil
}

// IsLoaded checks if a function is loaded.
func (l *FunctionLoader) IsLoaded(namespace, name string) bool {
	functionKey := GetFunctionKey(namespace, name)
	return l.pluginManager.IsPluginLoaded(functionKey)
}

// WasPreviouslyLoaded checks if a function was previously loaded.
func (l *FunctionLoader) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	functionKey := GetFunctionKey(namespace, name)
	return l.pluginManager.WasPreviouslyLoaded(functionKey)
}

// IsStopped checks if a function is stopped.
func (l *FunctionLoader) IsStopped(namespace, name string) bool {
	functionKey := GetFunctionKey(namespace, name)
	return l.pluginManager.IsFunctionStopped(functionKey)
}

// Helper methods

// pullWithContext fetches a WASM module with cancellation support
func (l *FunctionLoader) pullWithContext(ctx context.Context, namespace, name, identifier string) ([]byte, *registry.VersionInfo, error) {
	type pullResult struct {
		bytes []byte
		info  *registry.VersionInfo
	}

	// Create a wrapper function to use the shared utility
	wrapper := func() (pullResult, error) {
		bytes, info, err := l.registry.Pull(namespace, name, identifier)
		return pullResult{bytes, info}, err
	}

	// Execute with context handling
	result, err := utils.ExecuteWithContext(ctx, wrapper)
	if err != nil {
		return nil, nil, err
	}

	return result.bytes, result.info, nil
}

// createPluginWithContext creates a plugin with cancellation support
//
//nolint:whitespace // difficult to format exactly as linter expects
func (l *FunctionLoader) createPluginWithContext(ctx context.Context, wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (*extism.Plugin, error) {

	// Create a wrapper function to use the shared utility
	wrapper := func() (*extism.Plugin, error) {
		return components.CreatePlugin(wasmBytes, versionInfo, config)
	}

	// Execute with context cancellation handling
	plugin, err := utils.ExecuteWithContext(ctx, wrapper)

	// Clean up resources on error
	if err != nil && plugin != nil {
		plugin.Close(context.Background())
	}

	return plugin, err
}

// GetDigest returns the current digest of a function
func (l *FunctionLoader) GetDigest(namespace, name string) (string, bool) {
	functionKey := GetFunctionKey(namespace, name)
	return l.pluginManager.GetPluginDigest(functionKey)
}
