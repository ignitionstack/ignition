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
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	localRegistry "github.com/ignitionstack/ignition/pkg/registry/local"
	"github.com/ignitionstack/ignition/pkg/types"
)

// Alias logging levels for backward compatibility
const (
	LevelInfo    = logging.LevelInfo
	LevelWarning = logging.LevelWarning
	LevelError   = logging.LevelError
	LevelDebug   = logging.LevelDebug
)

// Engine is the main entrypoint for the engine package
// It implements the FunctionManager and RegistryOperator interfaces
type Engine struct {
	// Core dependencies
	registry       registry.Registry
	functionSvc    services.FunctionService
	defaultTimeout time.Duration
	logger         logging.Logger
	logStore       *logging.FunctionLogStore

	// Components
	pluginManager   *components.PluginManager
	circuitBreakers *components.CircuitBreakerManager

	// Function management abstractions
	functionManager FunctionManager
	functionLoader  *FunctionLoader
	functionExecutor *FunctionExecutor

	// Server configuration
	socketPath      string
	httpAddr        string
	initialized     bool
}

// NewEngine creates a new engine instance with default logger
func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	logger := logging.NewStdLogger(os.Stdout)
	return NewEngineWithLogger(socketPath, httpAddr, registryDir, logger)
}

// NewEngineWithLogger creates a new engine instance with custom logger
func NewEngineWithLogger(socketPath, httpAddr string, registryDir string, logger logging.Logger) (*Engine, error) {
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
	), nil
}

// NewEngineWithDependencies creates a new engine with custom dependencies
func NewEngineWithDependencies(
	socketPath,
	httpAddr string,
	registry registry.Registry,
	functionService services.FunctionService,
	logger logging.Logger,
) *Engine {
	defaultTimeout := 30 * time.Second
	logStore := logging.NewFunctionLogStore(1000)
	
	// Create core components
	pluginOptions := components.DefaultPluginOptions()
	pluginManager := components.NewPluginManager(logger, pluginOptions)
	circuitBreakerManager := components.NewCircuitBreakerManager()
	
	// Create function management abstractions
	functionLoader := NewFunctionLoader(registry, pluginManager, circuitBreakerManager, logStore, logger)
	functionExecutor := NewFunctionExecutor(pluginManager, circuitBreakerManager, logStore, logger, defaultTimeout)
	functionManager := NewFunctionManager(functionLoader, functionExecutor, registry, functionService, defaultTimeout)

	return &Engine{
		registry:        registry,
		functionSvc:     functionService,
		socketPath:      socketPath,
		httpAddr:        httpAddr,
		logger:          logger,
		initialized:     true,
		defaultTimeout:  defaultTimeout,
		logStore:        logStore,
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakerManager,
		functionLoader:  functionLoader,
		functionExecutor: functionExecutor,
		functionManager: functionManager,
	}
}

// setupRegistry initializes the registry with a badger database
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

// Start starts the engine and the HTTP server
func (e *Engine) Start() error {
	if !e.initialized {
		return ErrEngineNotInitialized
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the plugin manager's cleanup routine
	e.pluginManager.StartCleanup(ctx)

	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	return server.Start()
}

// FunctionManager interface implementation - delegate to the function manager

func (e *Engine) LoadFunctionWithContext(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config)
}

func (e *Engine) LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return e.functionManager.LoadFunctionWithForce(ctx, namespace, name, identifier, config, force)
}

func (e *Engine) CallFunctionWithContext(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	return e.functionManager.CallFunction(ctx, namespace, name, entrypoint, payload)
}

func (e *Engine) UnloadFunction(namespace, name string) error {
	return e.functionManager.UnloadFunction(namespace, name)
}

func (e *Engine) StopFunction(namespace, name string) error {
	return e.functionManager.StopFunction(namespace, name)
}

func (e *Engine) IsLoaded(namespace, name string) bool {
	return e.functionManager.IsLoaded(namespace, name)
}

func (e *Engine) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	return e.functionManager.WasPreviouslyLoaded(namespace, name)
}

func (e *Engine) IsFunctionStopped(namespace, name string) bool {
	return e.functionManager.IsStopped(namespace, name)
}

func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	return e.functionManager.BuildFunction(namespace, name, path, tag, config)
}

func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	return e.functionManager.ReassignTag(namespace, name, tag, newDigest)
}

// RegistryOperator interface implementation

func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}

// LoadFunctionWithContextAndForce is a backward-compatibility method
func (e *Engine) LoadFunctionWithContextAndForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	return e.LoadFunctionWithForce(ctx, namespace, name, identifier, config, force)
}

// Original method signatures needed for backward compatibility
func (e *Engine) LoadFunction(namespace, name, identifier string, config map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()
	return e.functionManager.LoadFunction(ctx, namespace, name, identifier, config)
}

func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()
	return e.functionManager.CallFunction(ctx, namespace, name, entrypoint, payload)
}