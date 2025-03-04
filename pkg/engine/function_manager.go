package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionManagerImpl implements the FunctionManager interface.
type FunctionManagerImpl struct {
	loader         *FunctionLoader
	executor       *FunctionExecutor
	registry       registry.Registry
	functionSvc    services.FunctionService
	defaultTimeout time.Duration
}

// NewFunctionManager creates a new function manager.
func NewFunctionManager(loader *FunctionLoader, executor *FunctionExecutor, registry registry.Registry,
	functionSvc services.FunctionService, defaultTimeout time.Duration) *FunctionManagerImpl {
	return &FunctionManagerImpl{
		loader:         loader,
		executor:       executor,
		registry:       registry,
		functionSvc:    functionSvc,
		defaultTimeout: defaultTimeout,
	}
}

// Core function operations

// LoadFunction loads a function with the specified identifier and configuration.
// If force is true, it will load even if the function is marked as stopped.
func (m *FunctionManagerImpl) LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return m.loader.LoadFunctionWithForce(ctx, namespace, name, identifier, config, force)
}

// CallFunction delegates to the executor
func (m *FunctionManagerImpl) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	return m.executor.CallFunction(ctx, namespace, name, entrypoint, payload)
}

// UnloadFunction delegates to the loader
func (m *FunctionManagerImpl) UnloadFunction(namespace, name string) error {
	return m.loader.UnloadFunction(namespace, name)
}

// StopFunction delegates to the loader
func (m *FunctionManagerImpl) StopFunction(namespace, name string) error {
	return m.loader.StopFunction(namespace, name)
}

// GetFunctionState returns the complete state information for a function
func (m *FunctionManagerImpl) GetFunctionState(namespace, name string) FunctionState {
	// Basic state
	isLoaded := m.loader.IsLoaded(namespace, name)
	isStopped := m.loader.IsStopped(namespace, name)
	wasLoaded, config := m.loader.WasPreviouslyLoaded(namespace, name)
	
	// Build the initial function state with core information
	state := FunctionState{
		Loaded:           isLoaded,
		Stopped:          isStopped,
		PreviouslyLoaded: wasLoaded,
		Config:           config,
	}
	
	// Only try to fetch additional information if we have a loaded function
	// or previously loaded function to avoid unnecessary operations
	if isLoaded || wasLoaded {
		// Try to get circuit breaker state if the function is loaded
		if isLoaded {
			functionKey := GetFunctionKey(namespace, name)
			cb := m.executor.circuitBreakers.GetCircuitBreaker(functionKey)
			if cb != nil {
				state.CircuitBreakerOpen = cb.IsOpen()
				state.Running = isLoaded && !cb.IsOpen()
			}
		}
		
		// Try to get digest from loader
		if digest, found := m.loader.GetDigest(namespace, name); found {
			state.Digest = digest
			
			// Get tags by enumerating all versions that match the digest
			metadata, err := m.registry.Get(namespace, name)
			if err == nil && metadata != nil {
				// Looking for all tags that point to this digest
				var tags []string
				for _, version := range metadata.Versions {
					if version.FullDigest == state.Digest {
						tags = append(tags, version.Tags...)
					}
				}
				state.Tags = tags
			}
		}
	}
	
	return state
}

// BuildFunction builds a function and stores it in the registry
func (m *FunctionManagerImpl) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
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
	buildResult, err := m.functionSvc.BuildFunction(path, config)
	if err != nil {
		return nil, fmt.Errorf("failed to build function: %w", err)
	}

	// Read the WASM file
	wasmBytes, err := os.ReadFile(buildResult.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	// Use digest as tag if none provided
	if tag == "" {
		tag = buildResult.Digest
	}

	// Store the function in the registry
	if err := m.registry.Push(namespace, name, wasmBytes, buildResult.Digest, tag, config.FunctionSettings.VersionSettings); err != nil {
		return nil, fmt.Errorf("failed to store in registry: %w", err)
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

// ReassignTag reassigns a tag to a different function version
func (m *FunctionManagerImpl) ReassignTag(namespace, name, tag, newDigest string) error {
	if err := m.registry.ReassignTag(namespace, name, tag, newDigest); err != nil {
		return fmt.Errorf("failed to reassign tag: %w", err)
	}
	return nil
}
