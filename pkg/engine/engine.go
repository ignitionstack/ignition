package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
)

// Engine represents the core service that manages WebAssembly functions
type Engine struct {
	registry    registry.Registry
	plugins     map[string]*extism.Plugin
	pluginsMux  sync.RWMutex
	socketPath  string
	httpAddr    string
	logger      Logger
	initialized bool
}

// NewEngine creates a new Engine instance with default logger
func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	// Create default logger
	logger := NewStdLogger(os.Stdout)

	return NewEngineWithLogger(socketPath, httpAddr, registryDir, logger)
}

// NewEngineWithLogger creates a new Engine instance with a custom logger
func NewEngineWithLogger(socketPath, httpAddr string, registryDir string, logger Logger) (*Engine, error) {
	// Setup registry
	registry, err := setupRegistry(registryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup registry: %w", err)
	}

	return &Engine{
		registry:    registry,
		plugins:     make(map[string]*extism.Plugin),
		socketPath:  socketPath,
		httpAddr:    httpAddr,
		logger:      logger,
		initialized: true,
	}, nil
}

// setupRegistry initializes and returns a registry instance
func setupRegistry(registryDir string) (registry.Registry, error) {
	opts := badger.DefaultOptions(filepath.Join(registryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	return localRegistry.NewLocalRegistry(registryDir, db), nil
}

// Start initializes and starts the engine's HTTP and socket servers
func (e *Engine) Start() error {
	if !e.initialized {
		return ErrEngineNotInitialized
	}

	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	return server.Start()
}

// getFunctionKey generates a consistent key for functions in the plugins map
func getFunctionKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// GetRegistry returns the registry associated with this engine
func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}

// CallFunction executes a previously loaded function with the given parameters
func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := getFunctionKey(namespace, name)

	e.pluginsMux.RLock()
	plugin, ok := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	if !ok {
		return nil, ErrFunctionNotLoaded
	}

	_, output, err := plugin.Call(entrypoint, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to call function: %w", err)
	}

	return output, nil
}

// LoadFunction loads a function from the registry into memory
func (e *Engine) LoadFunction(namespace, name, identifier string) error {
	e.logger.Printf("Loading function: %s/%s (identifier: %s)", namespace, name, identifier)

	// Get both the WASM bytes and version info
	wasmBytes, versionInfo, err := e.registry.Pull(namespace, name, identifier)
	if err != nil {
		e.logger.Errorf("Failed to fetch WASM file from registry: %v", err)
		return fmt.Errorf("failed to fetch WASM file from registry: %w", err)
	}

	// Create plugin from wasm bytes with appropriate settings
	plugin, err := createPlugin(wasmBytes, versionInfo)
	if err != nil {
		e.logger.Errorf("Failed to initialize plugin: %v", err)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Store the plugin
	e.pluginsMux.Lock()
	defer e.pluginsMux.Unlock()

	functionKey := getFunctionKey(namespace, name)
	e.plugins[functionKey] = plugin

	e.logger.Printf("Function loaded successfully: %s", functionKey)
	return nil
}

// createPlugin creates an Extism plugin from WASM bytes with version-specific settings
func createPlugin(wasmBytes []byte, versionInfo *registry.VersionInfo) (*extism.Plugin, error) {
	// Create the Extism manifest
	manifest := extism.Manifest{
		AllowedHosts: versionInfo.Settings.AllowedUrls,
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
	}

	// Apply version settings to plugin config
	config := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	// Initialize the plugin with version settings
	return extism.NewPlugin(context.Background(), manifest, config, []extism.HostFunction{})
}

// BuildFunction builds a function and stores it in the registry
func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*BuildResult, error) {
	e.logger.Printf("Building function: %s/%s", namespace, name)

	buildStart := time.Now()

	// Use default values if not provided
	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		name = filepath.Base(path)
	}

	// Build the function
	buildResult, err := buildFunction(path, config)
	if err != nil {
		e.logger.Errorf("Failed to build function: %v", err)
		return nil, fmt.Errorf("failed to build function: %w", err)
	}

	// Read the built wasm file
	wasmBytes, err := os.ReadFile(buildResult.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	// If no tag is provided, use the digest as the identifier
	if tag == "" {
		tag = buildResult.Digest
	}

	// Store in registry with version settings
	if err := e.registry.Push(namespace, name, wasmBytes, buildResult.Digest, tag, config.FunctionSettings.VersionSettings); err != nil {
		return nil, fmt.Errorf("failed to store in registry: %w", err)
	}

	e.logger.Printf("Function built successfully: %s/%s (digest: %s, tag: %s)",
		namespace, name, buildResult.Digest, tag)

	return &BuildResult{
		Name:      name,
		Namespace: namespace,
		Digest:    buildResult.Digest,
		BuildTime: time.Since(buildStart),
		Tag:       tag,
	}, nil
}

// buildFunction uses the function service to build a WASM module
func buildFunction(path string, config manifest.FunctionManifest) (*services.BuildResult, error) {
	service := services.NewFunctionService()
	return service.BuildFunction(path, config)
}

// ReassignTag updates a tag to point to a new digest
func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	e.logger.Printf("Reassigning tag %s to digest %s for function: %s/%s", tag, newDigest, namespace, name)

	// Reassign the tag in the registry
	if err := e.registry.ReassignTag(namespace, name, tag, newDigest); err != nil {
		e.logger.Errorf("Failed to reassign tag: %v", err)
		return fmt.Errorf("failed to reassign tag: %w", err)
	}

	e.logger.Printf("Tag %s reassigned to digest %s for function: %s/%s", tag, newDigest, namespace, name)
	return nil
}
