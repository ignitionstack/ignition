package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/internal/repository"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/config"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
	"github.com/ignitionstack/ignition/pkg/types"
)

// Alias logging levels for backward compatibility.
const (
	LevelInfo    = logging.LevelInfo
	LevelWarning = logging.LevelWarning
	LevelError   = logging.LevelError
	LevelDebug   = logging.LevelDebug
)

// It implements the FunctionManager and RegistryOperator interfaces.
type Engine struct {
	// Core dependencies
	registry       registry.Registry
	functionSvc    services.FunctionService
	defaultTimeout time.Duration
	logger         logging.Logger
	logStore       *logging.FunctionLogStore

	// Components
	pluginManager   PluginManager
	circuitBreakers CircuitBreakerManager

	// Function management abstractions
	functionManager  FunctionManager
	functionLoader   *FunctionLoader
	functionExecutor *FunctionExecutor

	// Server configuration
	socketPath  string
	httpAddr    string
	initialized bool

	// Configuration
	options *Options
	config  *config.Config
}

// NewEngine creates a new engine instance with default logger and options.
func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	logger := logging.NewStdLogger(os.Stdout)
	options := DefaultEngineOptions()
	return NewEngineWithOptions(socketPath, httpAddr, registryDir, logger, options)
}

// NewEngineWithLogger creates a new engine instance with custom logger.
func NewEngineWithLogger(socketPath, httpAddr string, registryDir string, logger logging.Logger) (*Engine, error) {
	options := DefaultEngineOptions()
	return NewEngineWithOptions(socketPath, httpAddr, registryDir, logger, options)
}

// NewEngineWithOptions creates a new engine instance with custom logger and options.
func NewEngineWithOptions(socketPath, httpAddr string, registryDir string, logger logging.Logger, options *Options) (*Engine, error) {
	registry, err := setupRegistry(registryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup registry: %w", err)
	}

	functionService := services.NewFunctionService()

	return NewEngineWithDependencies(
		socketPath,
		httpAddr,
		registry,
		functionService,
		logger,
		options,
	), nil
}

// NewEngineWithConfig creates a new engine instance using the provided configuration.
func NewEngineWithConfig(cfg *config.Config, logger logging.Logger) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	registry, err := setupRegistry(cfg.Server.RegistryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup registry: %w", err)
	}

	functionService := services.NewFunctionService()
	options := OptionsFromConfig(cfg)

	engine := NewEngineWithDependencies(
		cfg.Server.SocketPath,
		cfg.Server.HTTPAddr,
		registry,
		functionService,
		logger,
		options,
	)

	engine.config = cfg
	return engine, nil
}

// NewEngineWithDependencies creates a new engine with custom dependencies.
func NewEngineWithDependencies(
	socketPath,
	httpAddr string,
	registry registry.Registry,
	functionService services.FunctionService,
	logger logging.Logger,
	options *Options,
) *Engine {
	if options == nil {
		options = DefaultEngineOptions()
	}

	logStore := logging.NewFunctionLogStore(options.LogStoreCapacity)

	// Create core components
	pluginManager := components.NewPluginManager(logger, components.PluginManagerSettings{
		TTL:             options.PluginManagerSettings.TTL,
		CleanupInterval: options.PluginManagerSettings.CleanupInterval,
	})
	circuitBreakerManager := components.NewCircuitBreakerManagerWithOptions(options.CircuitBreakerSettings)

	// Create function management abstractions
	functionLoader := NewFunctionLoader(registry, pluginManager, circuitBreakerManager, logStore, logger)
	functionExecutor := NewFunctionExecutor(pluginManager, circuitBreakerManager, logStore, logger, options.DefaultTimeout)
	functionManager := NewFunctionManager(functionLoader, functionExecutor, registry, functionService, options.DefaultTimeout)

	return &Engine{
		registry:         registry,
		functionSvc:      functionService,
		socketPath:       socketPath,
		httpAddr:         httpAddr,
		logger:           logger,
		initialized:      true,
		defaultTimeout:   options.DefaultTimeout,
		logStore:         logStore,
		pluginManager:    pluginManager,
		circuitBreakers:  circuitBreakerManager,
		functionLoader:   functionLoader,
		functionExecutor: functionExecutor,
		functionManager:  functionManager,
		options:          options,
	}
}

// setupRegistry initializes the registry with a badger database.
func setupRegistry(registryDir string) (registry.Registry, error) {
	opts := badger.DefaultOptions(filepath.Join(registryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	dbRepo := repository.NewBadgerDBRepository(db)
	return localRegistry.NewLocalRegistry(registryDir, dbRepo), nil
}

// GetConfig returns the engine's configuration.
func (e *Engine) GetConfig() *config.Config {
	return e.config
}

// Start initializes the engine components and starts the HTTP server.
//
// The server will continue running until terminated or an error occurs.
//
// Returns:
//   - error: Any error that occurred during startup
func (e *Engine) Start() error {
	// Validate engine state
	if err := e.validateState(); err != nil {
		return err
	}

	// Create context for cleanup routines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	e.initializeComponents(ctx)

	// Set up and start the server
	return e.startServer()
}

// validateState ensures the engine is properly initialized.
func (e *Engine) validateState() error {
	if !e.initialized {
		return ErrEngineNotInitialized
	}
	return nil
}

// initializeComponents starts all background processes and cleanup routines.
func (e *Engine) initializeComponents(ctx context.Context) {
	// Start the plugin manager's cleanup routine
	e.pluginManager.StartCleanup(ctx)
}

// startServer creates and starts the HTTP server.
func (e *Engine) startServer() error {
	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	e.logger.Printf("Starting Ignition engine server on socket %s and HTTP %s", e.socketPath, e.httpAddr)
	return server.Start()
}

// FunctionManager interface implementation - methods below delegate to the function manager

// LoadFunctionWithContext loads a function with the specified identifier and configuration.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - namespace: The function namespace
//   - name: The function name
//   - identifier: The function identifier (digest or tag)
//   - config: Configuration values for the function
//
// Returns:
//   - error: Any error that occurred during loading
func (e *Engine) LoadFunctionWithContext(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config)
}

// LoadFunctionWithForce loads a function with the option to force loading even if stopped.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - namespace: The function namespace
//   - name: The function name
//   - identifier: The function identifier (digest or tag)
//   - config: Configuration values for the function
//   - force: Whether to force loading even if the function is marked as stopped
//
// Returns:
//   - error: Any error that occurred during loading
func (e *Engine) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return e.functionManager.LoadFunctionWithForce(ctx, namespace, name, identifier, config, force)
}

// CallFunctionWithContext calls a function with the specified parameters.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - namespace: The function namespace
//   - name: The function name
//   - entrypoint: The entry point function to call
//   - payload: The input payload for the function
//
// Returns:
//   - []byte: The output from the function call
//   - error: Any error that occurred during execution
func (e *Engine) CallFunctionWithContext(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	return e.functionManager.CallFunction(ctx, namespace, name, entrypoint, payload)
}

// UnloadFunction unloads a function, removing it from memory but preserving its configuration.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - error: Any error that occurred during unloading
func (e *Engine) UnloadFunction(namespace, name string) error {
	return e.functionManager.UnloadFunction(namespace, name)
}

// StopFunction stops a function and marks it as explicitly stopped to prevent auto-reload.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - error: Any error that occurred during stopping
func (e *Engine) StopFunction(namespace, name string) error {
	return e.functionManager.StopFunction(namespace, name)
}

// IsLoaded checks if a function is currently loaded.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - bool: True if the function is loaded, false otherwise
func (e *Engine) IsLoaded(namespace, name string) bool {
	return e.functionManager.IsLoaded(namespace, name)
}

// WasPreviouslyLoaded checks if a function was previously loaded and returns its config.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - bool: True if the function was previously loaded
//   - map[string]string: The function's last known configuration
func (e *Engine) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	return e.functionManager.WasPreviouslyLoaded(namespace, name)
}

// IsFunctionStopped checks if a function is explicitly stopped.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//
// Returns:
//   - bool: True if the function is stopped, false otherwise
func (e *Engine) IsFunctionStopped(namespace, name string) bool {
	return e.functionManager.IsStopped(namespace, name)
}

// BuildFunction builds a function and stores it in the registry.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//   - path: The path to the function source code
//   - tag: The tag to assign to the built function
//   - config: The function manifest configuration
//
// Returns:
//   - *types.BuildResult: The result of the build operation
//   - error: Any error that occurred during building
func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	return e.functionManager.BuildFunction(namespace, name, path, tag, config)
}

// ReassignTag reassigns a tag to a different function version.
//
// Parameters:
//   - namespace: The function namespace
//   - name: The function name
//   - tag: The tag to reassign
//   - newDigest: The digest of the version to assign the tag to
//
// Returns:
//   - error: Any error that occurred during the operation
func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	return e.functionManager.ReassignTag(namespace, name, tag, newDigest)
}

// RegistryOperator interface implementation

// GetRegistry returns the registry instance used by the engine.
//
// Returns:
//   - registry.Registry: The registry instance
func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}
