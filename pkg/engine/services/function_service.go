package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine/errors"
	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/engine/wasm"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionServiceImpl implements interfaces.FunctionService
type FunctionServiceImpl struct {
	stateManager     interfaces.StateManager
	executionService interfaces.ExecutionService
	registry         registry.Registry
	functionSvc      services.FunctionService
	logger           logging.Logger
	logStore         *logging.FunctionLogStore
	keyHandler       interfaces.KeyHandler
	runtimeFactory   interfaces.WasmRuntimeFactory
	circuitBreaker   interfaces.CircuitBreakerManager
	defaultTimeout   time.Duration
}

// NewFunctionService creates a new FunctionServiceImpl
func NewFunctionService(
	stateManager interfaces.StateManager,
	executionService interfaces.ExecutionService,
	registry registry.Registry,
	functionSvc services.FunctionService,
	logger logging.Logger,
	logStore *logging.FunctionLogStore,
	keyHandler interfaces.KeyHandler,
	runtimeFactory interfaces.WasmRuntimeFactory,
	circuitBreaker interfaces.CircuitBreakerManager,
	defaultTimeout time.Duration,
) *FunctionServiceImpl {
	return &FunctionServiceImpl{
		stateManager:     stateManager,
		executionService: executionService,
		registry:         registry,
		functionSvc:      functionSvc,
		logger:           logger,
		logStore:         logStore,
		keyHandler:       keyHandler,
		runtimeFactory:   runtimeFactory,
		circuitBreaker:   circuitBreaker,
		defaultTimeout:   defaultTimeout,
	}
}

// LoadFunction implements interfaces.FunctionService
func (s *FunctionServiceImpl) LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	key := s.keyHandler.GetKey(namespace, name)
	s.logger.Printf("Loading function: %s (identifier: %s, force: %v)", key, identifier, force)

	// Check if function is stopped and force is not set
	if s.stateManager.IsStopped(namespace, name) && !force {
		errMsg := fmt.Sprintf("Function %s is stopped and cannot be loaded without force option", key)
		s.logger.Printf(errMsg)
		s.logStore.AddLog(key, logging.LevelError,
			"Cannot load stopped function. Use 'ignition function run' to explicitly load it")
		return errors.Wrap(errors.DomainFunction, errors.CodeFunctionStopped,
			"function was explicitly stopped - use 'ignition function run' to load it", nil).
			WithNamespace(namespace).
			WithName(name)
	}

	// If force-loading a stopped function, clear the stopped status
	if s.stateManager.IsStopped(namespace, name) && force {
		s.logger.Printf("Force loading stopped function %s - clearing stopped status", key)
		s.logStore.AddLog(key, logging.LevelInfo, "Force loading stopped function - clearing stopped status")
		s.stateManager.ClearStoppedStatus(namespace, name)
	}

	s.logStore.AddLog(key, logging.LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	// Create a deep copy of the config map to prevent side effects
	configCopy := s.copyConfig(config)

	// Fetch the WASM bytes from the registry
	loadStart := time.Now()
	wasmBytes, versionInfo, err := s.registry.Pull(namespace, name, identifier)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to fetch WASM file from registry: %v", err)
		s.logger.Errorf(errMsg)
		s.logStore.AddLog(key, logging.LevelError, errMsg)

		if err == registry.ErrFunctionNotFound || err == registry.ErrVersionNotFound {
			return errors.Wrap(errors.DomainRegistry, errors.CodeFunctionNotFound,
				"Function or version not found in registry", err).
				WithNamespace(namespace).
				WithName(name)
		}

		return errors.Wrap(errors.DomainRegistry, errors.CodeRegistryError,
			"Failed to fetch function from registry", err).
			WithNamespace(namespace).
			WithName(name)
	}

	// Log success and record detailed information
	s.logStore.AddLog(key, logging.LevelInfo,
		fmt.Sprintf("Function pulled from registry (size: %d bytes, time: %v)",
			len(wasmBytes), time.Since(loadStart)))

	actualDigest := versionInfo.FullDigest

	// Handle existing function if loaded
	if s.stateManager.IsLoaded(namespace, name) {
		// Check if anything has changed
		currentDigest, _ := s.stateManager.GetDigest(namespace, name)

		// If nothing changed, we can skip reloading
		if currentDigest == actualDigest {
			s.logger.Printf("Function %s already loaded with same digest", key)
			s.logStore.AddLog(key, logging.LevelInfo, "Function already loaded with same digest")
			return nil
		}

		// Log what changed
		s.logger.Printf("Function %s digest changed from %s to %s, reloading",
			key, currentDigest, actualDigest)
		s.logStore.AddLog(key, logging.LevelInfo,
			fmt.Sprintf("Function digest changed from %s to %s, reloading",
				currentDigest, actualDigest))

		// Get existing runtime and close it
		if runtime, exists := s.stateManager.GetRuntime(namespace, name); exists {
			if err := runtime.Close(ctx); err != nil {
				s.logger.Printf("Warning: Error closing existing runtime: %v", err)
			}
		}

		// Remove existing runtime and circuit breaker
		s.stateManager.RemoveRuntime(namespace, name)
		s.circuitBreaker.Reset(namespace, name)
	}

	// Create and initialize the runtime
	return s.createAndStoreRuntime(ctx, namespace, name, wasmBytes, versionInfo, configCopy)
}

// UnloadFunction implements interfaces.FunctionService
func (s *FunctionServiceImpl) UnloadFunction(namespace, name string) error {
	key := s.keyHandler.GetKey(namespace, name)
	s.logger.Printf("Unloading function: %s", key)
	s.logStore.AddLog(key, logging.LevelInfo, "Unloading function")

	// Check if the function is loaded
	if !s.stateManager.IsLoaded(namespace, name) {
		notLoadedMsg := fmt.Sprintf("Function %s is not loaded, nothing to unload", key)
		s.logger.Printf(notLoadedMsg)
		s.logStore.AddLog(key, logging.LevelInfo, notLoadedMsg)
		return nil
	}

	// Track performance
	unloadStart := time.Now()

	// Close the runtime if it exists
	if runtime, exists := s.stateManager.GetRuntime(namespace, name); exists {
		if err := runtime.Close(context.Background()); err != nil {
			s.logger.Printf("Warning: Error closing runtime: %v", err)
		}
	}

	// Remove runtime and update state
	s.stateManager.RemoveRuntime(namespace, name)
	s.stateManager.MarkUnloaded(namespace, name)

	// Remove circuit breaker
	s.circuitBreaker.Reset(namespace, name)

	// Log success
	successMsg := fmt.Sprintf("Function %s unloaded successfully (time: %v)", key, time.Since(unloadStart))
	s.logger.Printf(successMsg)
	s.logStore.AddLog(key, logging.LevelInfo, successMsg)
	s.logStore.AddLog(key, logging.LevelInfo, "Function unloaded - this is the final log entry")

	return nil
}

// StopFunction implements interfaces.FunctionService
func (s *FunctionServiceImpl) StopFunction(namespace, name string) error {
	key := s.keyHandler.GetKey(namespace, name)
	s.logger.Printf("Stopping function: %s", key)
	s.logStore.AddLog(key, logging.LevelInfo, "Stopping function")

	// Check if the function is already stopped
	if s.stateManager.IsStopped(namespace, name) {
		alreadyStoppedMsg := fmt.Sprintf("Function %s is already stopped", key)
		s.logger.Printf(alreadyStoppedMsg)
		s.logStore.AddLog(key, logging.LevelInfo, alreadyStoppedMsg)
		return nil
	}

	// Track performance
	stopStart := time.Now()

	// Close the runtime if it exists
	if runtime, exists := s.stateManager.GetRuntime(namespace, name); exists {
		if err := runtime.Close(context.Background()); err != nil {
			s.logger.Printf("Warning: Error closing runtime: %v", err)
		}
	}

	// Remove runtime and update state
	s.stateManager.RemoveRuntime(namespace, name)
	s.stateManager.MarkStopped(namespace, name)

	// Remove circuit breaker
	s.circuitBreaker.Reset(namespace, name)

	// Log success
	successMsg := fmt.Sprintf("Function %s stopped successfully (time: %v)", key, time.Since(stopStart))
	s.logger.Printf(successMsg)
	s.logStore.AddLog(key, logging.LevelInfo, successMsg)
	s.logStore.AddLog(key, logging.LevelInfo, "Function stopped - will not be automatically reloaded")

	return nil
}

// CallFunction implements interfaces.FunctionService
func (s *FunctionServiceImpl) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	// Execute the function
	result, err := s.executionService.Execute(ctx, interfaces.ExecutionParams{
		Namespace:  namespace,
		Name:       name,
		Entrypoint: entrypoint,
		Payload:    payload,
	})

	if err != nil {
		return nil, err
	}

	return result.Output, nil
}

// GetFunctionState implements interfaces.FunctionService
func (s *FunctionServiceImpl) GetFunctionState(namespace, name string) interfaces.FunctionState {
	return s.stateManager.GetState(namespace, name)
}

// BuildFunction implements interfaces.FunctionService
func (s *FunctionServiceImpl) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	// Track build time
	buildStart := time.Now()

	// Apply default values if not provided
	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		name = filepath.Base(path)
	}

	// Build the function
	buildResult, err := s.functionSvc.BuildFunction(path, config)
	if err != nil {
		return nil, errors.Wrap(errors.DomainFunction, errors.CodeInternalError,
			"Failed to build function", err).
			WithNamespace(namespace).
			WithName(name)
	}

	// Read the WASM file
	wasmBytes, err := os.ReadFile(buildResult.Path)
	if err != nil {
		return nil, errors.Wrap(errors.DomainFunction, errors.CodeInternalError,
			"Failed to read wasm file", err).
			WithNamespace(namespace).
			WithName(name)
	}

	// Use digest as tag if none provided
	if tag == "" {
		tag = buildResult.Digest
	}

	// Store the function in the registry
	if err := s.registry.Push(namespace, name, wasmBytes, buildResult.Digest, tag, config.FunctionSettings.VersionSettings); err != nil {
		return nil, errors.Wrap(errors.DomainRegistry, errors.CodeRegistryError,
			"Failed to store in registry", err).
			WithNamespace(namespace).
			WithName(name)
	}

	// Return build result
	return &types.BuildResult{
		Name:      name,
		Namespace: namespace,
		Digest:    buildResult.Digest,
		BuildTime: time.Since(buildStart).String(),
		Tag:       tag,
		Reused:    false, // By default mark as not reused
	}, nil
}

// ReassignTag implements interfaces.FunctionService
func (s *FunctionServiceImpl) ReassignTag(namespace, name, tag, newDigest string) error {
	if err := s.registry.ReassignTag(namespace, name, tag, newDigest); err != nil {
		return errors.Wrap(errors.DomainRegistry, errors.CodeRegistryError,
			"Failed to reassign tag", err).
			WithNamespace(namespace).
			WithName(name)
	}
	return nil
}

// Helper methods

// copyConfig creates a deep copy of a configuration map
func (s *FunctionServiceImpl) copyConfig(config map[string]string) map[string]string {
	if config == nil {
		return nil
	}

	configCopy := make(map[string]string, len(config))
	for k, v := range config {
		configCopy[k] = v
	}
	return configCopy
}

// createAndStoreRuntime creates a new runtime instance and stores it in the state manager
func (s *FunctionServiceImpl) createAndStoreRuntime(
	ctx context.Context,
	namespace, name string,
	wasmBytes []byte,
	versionInfo *registry.VersionInfo,
	config map[string]string,
) error {
	key := s.keyHandler.GetKey(namespace, name)

	// Create a new runtime instance
	initStart := time.Now()
	runtime, err := wasm.CreateExtismRuntimeFromVersionInfo(wasmBytes, versionInfo, config)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to initialize runtime: %v", err)
		s.logger.Errorf(errMsg)
		s.logStore.AddLog(key, logging.LevelError, errMsg)
		return errors.Wrap(errors.DomainPlugin, errors.CodePluginCreationFailed,
			"Failed to initialize runtime", err).
			WithNamespace(namespace).
			WithName(name)
	}

	// Log successful initialization
	s.logStore.AddLog(key, logging.LevelInfo,
		fmt.Sprintf("Runtime initialized successfully (time: %v)", time.Since(initStart)))

	// Store the runtime and update state
	s.stateManager.StoreRuntime(namespace, name, runtime)
	s.stateManager.MarkLoaded(namespace, name, versionInfo.FullDigest, config)

	// Log success
	successMsg := fmt.Sprintf("Function loaded successfully: %s", key)
	s.logger.Printf(successMsg)
	s.logStore.AddLog(key, logging.LevelInfo, successMsg)

	return nil
}
