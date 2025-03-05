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

const (
	LevelInfo    = logging.LevelInfo
	LevelWarning = logging.LevelWarning
	LevelError   = logging.LevelError
	LevelDebug   = logging.LevelDebug
)

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

// LoadFunctionWithContext loads a function with the specified identifier and configuration.
func (e *Engine) LoadFunctionWithContext(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config, false)
}

// LoadFunctionWithForce loads a function with the option to force loading even if stopped.
func (e *Engine) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config, force)
}

// CallFunctionWithContext calls a function with the specified parameters.
func (e *Engine) CallFunctionWithContext(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	return e.functionManager.CallFunction(ctx, namespace, name, entrypoint, payload)
}

// UnloadFunction unloads a function, removing it from memory but preserving its configuration.
func (e *Engine) UnloadFunction(namespace, name string) error {
	return e.functionManager.UnloadFunction(namespace, name)
}

// StopFunction stops a function and marks it as explicitly stopped to prevent auto-reload.
func (e *Engine) StopFunction(namespace, name string) error {
	return e.functionManager.StopFunction(namespace, name)
}

// IsLoaded checks if a function is currently loaded.
func (e *Engine) IsLoaded(namespace, name string) bool {
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.Loaded
}

// WasPreviouslyLoaded checks if a function was previously loaded and returns its config.
func (e *Engine) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.PreviouslyLoaded, state.Config
}

// IsFunctionStopped checks if a function is explicitly stopped.
func (e *Engine) IsFunctionStopped(namespace, name string) bool {
	state := e.functionManager.GetFunctionState(namespace, name)
	return state.Stopped
}

// BuildFunction builds a function and stores it in the registry.
func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	return e.functionManager.BuildFunction(namespace, name, path, tag, config)
}

// ReassignTag reassigns a tag to a different function version.
func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	return e.functionManager.ReassignTag(namespace, name, tag, newDigest)
}

// GetRegistry returns the registry instance.
// This is a convenience method for direct access when needed.
func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}
