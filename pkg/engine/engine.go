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

// Log levels in the engine
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

// NewEngine creates a new engine instance with default settings.
// It's a convenient way to create an engine with minimal configuration.
func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	return NewEngineWithOptions(socketPath, httpAddr, registryDir, nil, nil)
}

// NewEngineWithOptions creates a new engine instance with custom settings.
// Accepts optional logger and options parameters (nil values use defaults).
func NewEngineWithOptions(socketPath, httpAddr string, registryDir string,
	logger logging.Logger, options *Options) (*Engine, error) {
	// Use defaults for nil parameters
	if logger == nil {
		logger = logging.NewStdLogger(os.Stdout)
	}
	if options == nil {
		options = DefaultEngineOptions()
	}

	// Setup the registry
	registry, err := setupRegistry(registryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup registry: %w", err)
	}

	// Create function service
	functionService := services.NewFunctionService()

	// Create common components
	logStore := logging.NewFunctionLogStore(options.LogStoreCapacity)
	pluginManager := components.NewPluginManager(logger, components.PluginManagerSettings{
		TTL:             options.PluginManagerSettings.TTL,
		CleanupInterval: options.PluginManagerSettings.CleanupInterval,
	})
	circuitBreakerManager := components.NewCircuitBreakerManagerWithOptions(options.CircuitBreakerSettings)

	// Create function management components
	functionLoader := NewFunctionLoader(registry, pluginManager, circuitBreakerManager, logStore, logger)
	functionExecutor := NewFunctionExecutor(pluginManager, circuitBreakerManager, logStore, logger, options.DefaultTimeout)
	functionManager := NewFunctionManager(functionLoader, functionExecutor, registry, functionService, options.DefaultTimeout)

	// Assemble the engine
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
	}, nil
}

// NewEngineWithConfig creates a new engine instance using a configuration object.
func NewEngineWithConfig(cfg *config.Config, logger logging.Logger) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	// Use default logger if none provided
	if logger == nil {
		logger = logging.NewStdLogger(os.Stdout)
	}

	// Create options from config
	options := OptionsFromConfig(cfg)

	// Create engine with config-derived settings
	engine, err := NewEngineWithOptions(
		cfg.Server.SocketPath,
		cfg.Server.HTTPAddr,
		cfg.Server.RegistryDir,
		logger,
		options,
	)

	if err != nil {
		return nil, err
	}

	// Store the config for reference
	engine.config = cfg
	return engine, nil
}

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

func (e *Engine) GetConfig() *config.Config {
	return e.config
}

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

func (e *Engine) validateState() error {
	if !e.initialized {
		return ErrEngineNotInitialized
	}
	return nil
}

func (e *Engine) initializeComponents(ctx context.Context) {
	// Start the plugin manager's cleanup routine
	e.pluginManager.StartCleanup(ctx)
}

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
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config, false)
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
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config, force)
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
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.Loaded
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
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.PreviouslyLoaded, state.Config
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
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.Stopped
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

// GetRegistry returns the registry instance.
// This is a convenience method for direct access when needed.
//
// Returns:
//   - registry.Registry: The registry instance
func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}
