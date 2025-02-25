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
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
)

// CircuitBreaker manages function reliability
type CircuitBreaker struct {
	failures         int
	lastFailure      time.Time
	state            string // "closed", "open", "half-open"
	failureThreshold int
	resetTimeout     time.Duration
	mutex            sync.RWMutex
}

// Engine represents the core service that manages WebAssembly functions
type Engine struct {
	registry        registry.Registry
	functionService services.FunctionService
	plugins         map[string]*extism.Plugin
	pluginsMux      sync.RWMutex
	socketPath      string
	httpAddr        string
	logger          Logger
	initialized     bool

	// TTL-based plugin management
	pluginLastUsed map[string]time.Time
	ttlDuration    time.Duration
	cleanupTicker  *time.Ticker

	// Timeout handling
	defaultTimeout time.Duration

	// Circuit breaking
	circuitBreakers map[string]*CircuitBreaker
	cbMux           sync.RWMutex
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

	// Create function service
	functionService := services.NewFunctionService()

	return NewEngineWithDependencies(
		socketPath,
		httpAddr,
		registry,
		functionService,
		logger,
	), nil
}

// NewEngineWithDependencies creates a new Engine instance with explicit dependencies
func NewEngineWithDependencies(
	socketPath,
	httpAddr string,
	registry registry.Registry,
	functionService services.FunctionService,
	logger Logger,
) *Engine {
	return &Engine{
		registry:        registry,
		functionService: functionService,
		plugins:         make(map[string]*extism.Plugin),
		socketPath:      socketPath,
		httpAddr:        httpAddr,
		logger:          logger,
		initialized:     true,

		// TTL-based plugin management
		pluginLastUsed: make(map[string]time.Time),
		ttlDuration:    30 * time.Minute,

		// Timeout handling
		defaultTimeout: 30 * time.Second,

		// Circuit breaking
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// setupRegistry initializes and returns a registry instance
func setupRegistry(registryDir string) (registry.Registry, error) {
	opts := badger.DefaultOptions(filepath.Join(registryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	// Create a DB repository
	dbRepo := repository.NewBadgerDBRepository(db)

	// Create and return registry with the repository
	return localRegistry.NewLocalRegistry(registryDir, dbRepo), nil
}

// Start initializes and starts the engine's HTTP and socket servers
func (e *Engine) Start() error {
	if !e.initialized {
		return ErrEngineNotInitialized
	}

	// Start TTL-based plugin cleanup
	e.cleanupTicker = time.NewTicker(5 * time.Minute)
	go e.cleanupUnusedPlugins()

	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	return server.Start()
}

// cleanupUnusedPlugins periodically removes unused plugins to prevent memory leaks
func (e *Engine) cleanupUnusedPlugins() {
	for range e.cleanupTicker.C {
		e.pluginsMux.Lock()
		now := time.Now()
		for key, lastUsed := range e.pluginLastUsed {
			if now.Sub(lastUsed) > e.ttlDuration {
				if plugin, exists := e.plugins[key]; exists {
					plugin.Close(context.TODO())
					delete(e.plugins, key)
					delete(e.pluginLastUsed, key)
					e.logger.Printf("Plugin %s unloaded due to inactivity", key)
				}
			}
		}
		e.pluginsMux.Unlock()
	}
}

// getFunctionKey generates a consistent key for functions in the plugins map
func getFunctionKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// GetRegistry returns the registry associated with this engine
func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}

// newCircuitBreaker creates a new circuit breaker with default settings
func newCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		failures:         0,
		state:            "closed",
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

// recordSuccess records a successful function call
func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == "half-open" {
		cb.failures = 0
		cb.state = "closed"
	}
}

// recordFailure records a function failure and returns whether circuit is now open
func (cb *CircuitBreaker) recordFailure() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == "closed" && cb.failures >= cb.failureThreshold {
		cb.state = "open"
	}

	return cb.state == "open"
}

// isOpen checks if the circuit breaker is open
func (cb *CircuitBreaker) isOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if cb.state == "open" {
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			// Allow one test request
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.state = "half-open"
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return false
		}
		return true
	}

	return false
}

// CallFunction executes a previously loaded function with the given parameters
func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := getFunctionKey(namespace, name)

	// Check circuit breaker
	e.cbMux.RLock()
	cb, cbExists := e.circuitBreakers[functionKey]
	e.cbMux.RUnlock()

	if !cbExists {
		e.cbMux.Lock()
		cb = newCircuitBreaker()
		e.circuitBreakers[functionKey] = cb
		e.cbMux.Unlock()
	}

	if cb.isOpen() {
		return nil, fmt.Errorf("circuit breaker is open for function %s", functionKey)
	}

	// Update last used timestamp
	e.pluginsMux.RLock()
	plugin, ok := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	if ok {
		e.pluginsMux.Lock()
		e.pluginLastUsed[functionKey] = time.Now()
		e.pluginsMux.Unlock()
	}

	if !ok {
		return nil, ErrFunctionNotLoaded
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()

	// Create channel for results
	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	// Execute function in goroutine
	go func() {
		_, output, err := plugin.Call(entrypoint, payload)
		resultCh <- struct {
			output []byte
			err    error
		}{output, err}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		if result.err != nil {
			isOpen := cb.recordFailure()
			if isOpen {
				e.logger.Printf("Circuit breaker opened for function %s", functionKey)
			}
			return nil, fmt.Errorf("failed to call function: %w", result.err)
		}

		cb.recordSuccess()
		return result.output, nil

	case <-ctx.Done():
		isOpen := cb.recordFailure()
		if isOpen {
			e.logger.Printf("Circuit breaker opened for function %s", functionKey)
		}
		return nil, fmt.Errorf("function execution timed out after %v", e.defaultTimeout)
	}
}

// LoadFunction loads a function from the registry into memory
func (e *Engine) LoadFunction(namespace, name, identifier string) error {
	e.logger.Printf("Loading function: %s/%s (identifier: %s)", namespace, name, identifier)

	functionKey := getFunctionKey(namespace, name)

	// Check if function is already loaded
	e.pluginsMux.RLock()
	_, alreadyLoaded := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	if alreadyLoaded {
		e.logger.Printf("Function %s already loaded", functionKey)

		// Update timestamp
		e.pluginsMux.Lock()
		e.pluginLastUsed[functionKey] = time.Now()
		e.pluginsMux.Unlock()

		return nil
	}

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

	// Double-check that it wasn't loaded while we were fetching
	if _, exists := e.plugins[functionKey]; exists {
		// Another goroutine loaded it already, close our copy
		plugin.Close(context.TODO())
		return nil
	}

	e.plugins[functionKey] = plugin
	e.pluginLastUsed[functionKey] = time.Now()

	// Initialize circuit breaker
	e.cbMux.Lock()
	if _, exists := e.circuitBreakers[functionKey]; !exists {
		e.circuitBreakers[functionKey] = newCircuitBreaker()
	}
	e.cbMux.Unlock()

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

	// Build the function using the injected function service
	buildResult, err := e.functionService.BuildFunction(path, config)
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

// This standalone buildFunction has been removed in favor of using the injected functionService

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
