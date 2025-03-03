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

// DefaultFunctionManager implements the FunctionManager interface
type DefaultFunctionManager struct {
	loader         *FunctionLoader
	executor       *FunctionExecutor
	registry       registry.Registry
	functionSvc    services.FunctionService
	defaultTimeout time.Duration
}

// NewFunctionManager creates a new function manager
func NewFunctionManager(loader *FunctionLoader, executor *FunctionExecutor, registry registry.Registry, 
	functionSvc services.FunctionService, defaultTimeout time.Duration) *DefaultFunctionManager {
	return &DefaultFunctionManager{
		loader:         loader,
		executor:       executor,
		registry:       registry,
		functionSvc:    functionSvc,
		defaultTimeout: defaultTimeout,
	}
}

// LoadFunction loads a function
func (m *DefaultFunctionManager) LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return m.loader.LoadFunction(ctx, namespace, name, identifier, config)
}

// LoadFunctionWithForce loads a function with force option
func (m *DefaultFunctionManager) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return m.loader.LoadFunctionWithForce(ctx, namespace, name, identifier, config, force)
}

// CallFunction calls a function
func (m *DefaultFunctionManager) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	return m.executor.CallFunction(ctx, namespace, name, entrypoint, payload)
}

// UnloadFunction unloads a function
func (m *DefaultFunctionManager) UnloadFunction(namespace, name string) error {
	return m.loader.UnloadFunction(namespace, name)
}

// StopFunction stops a function
func (m *DefaultFunctionManager) StopFunction(namespace, name string) error {
	return m.loader.StopFunction(namespace, name)
}

// IsLoaded checks if a function is loaded
func (m *DefaultFunctionManager) IsLoaded(namespace, name string) bool {
	return m.loader.IsLoaded(namespace, name)
}

// WasPreviouslyLoaded checks if a function was previously loaded
func (m *DefaultFunctionManager) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	return m.loader.WasPreviouslyLoaded(namespace, name)
}

// IsStopped checks if a function is stopped
func (m *DefaultFunctionManager) IsStopped(namespace, name string) bool {
	return m.loader.IsStopped(namespace, name)
}

// BuildFunction builds a function
func (m *DefaultFunctionManager) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	buildStart := time.Now()

	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		name = filepath.Base(path)
	}

	buildResult, err := m.functionSvc.BuildFunction(path, config)
	if err != nil {
		return nil, fmt.Errorf("failed to build function: %w", err)
	}

	wasmBytes, err := os.ReadFile(buildResult.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	if tag == "" {
		tag = buildResult.Digest
	}

	if err := m.registry.Push(namespace, name, wasmBytes, buildResult.Digest, tag, config.FunctionSettings.VersionSettings); err != nil {
		return nil, fmt.Errorf("failed to store in registry: %w", err)
	}

	return &types.BuildResult{
		Name:      name,
		Namespace: namespace,
		Digest:    buildResult.Digest,
		BuildTime: time.Since(buildStart),
		Tag:       tag,
	}, nil
}

// ReassignTag reassigns a tag to a new digest
func (m *DefaultFunctionManager) ReassignTag(namespace, name, tag, newDigest string) error {
	if err := m.registry.ReassignTag(namespace, name, tag, newDigest); err != nil {
		return fmt.Errorf("failed to reassign tag: %w", err)
	}
	return nil
}