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

type Engine struct {
	registry        registry.Registry
	functionService services.FunctionService
	socketPath      string
	httpAddr        string
	logger          logging.Logger
	initialized     bool
	defaultTimeout  time.Duration
	logStore        *logging.FunctionLogStore

	pluginManager   *components.PluginManager
	circuitBreakers *components.CircuitBreakerManager
}

func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	logger := logging.NewStdLogger(os.Stdout)
	return NewEngineWithLogger(socketPath, httpAddr, registryDir, logger)
}

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

func NewEngineWithDependencies(
	socketPath,
	httpAddr string,
	registry registry.Registry,
	functionService services.FunctionService,
	logger logging.Logger,
) *Engine {
	pluginOptions := components.DefaultPluginOptions()
	pluginManager := components.NewPluginManager(logger, pluginOptions)
	circuitBreakerManager := components.NewCircuitBreakerManager()

	return &Engine{
		registry:        registry,
		functionService: functionService,
		socketPath:      socketPath,
		httpAddr:        httpAddr,
		logger:          logger,
		initialized:     true,
		defaultTimeout:  30 * time.Second,
		logStore:        logging.NewFunctionLogStore(1000),
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakerManager,
	}
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

func (e *Engine) IsLoaded(namespace, name string) bool {
	functionKey := components.GetFunctionKey(namespace, name)
	return e.pluginManager.IsPluginLoaded(functionKey)
}

func (e *Engine) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	functionKey := components.GetFunctionKey(namespace, name)
	return e.pluginManager.WasPreviouslyLoaded(functionKey)
}

func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}

func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes", entrypoint, len(payload)))

	cb := e.circuitBreakers.GetCircuitBreaker(functionKey)
	if cb.IsOpen() {
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", functionKey)
		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	plugin, ok := e.pluginManager.GetPlugin(functionKey)
	if !ok {
		e.logStore.AddLog(functionKey, logging.LevelError, "Function not loaded")
		return nil, ErrFunctionNotLoaded
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()

	startTime := time.Now()

	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	go func() {
		_, output, err := plugin.Call(entrypoint, payload)
		resultCh <- struct {
			output []byte
			err    error
		}{output, err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			isOpen := cb.RecordFailure()
			errMsg := fmt.Sprintf("Failed to call function: %v", result.err)
			e.logStore.AddLog(functionKey, LevelError, errMsg)

			if isOpen {
				cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
				e.logger.Printf(cbMsg)
				e.logStore.AddLog(functionKey, LevelError, cbMsg)
			}

			return nil, fmt.Errorf("failed to call function: %w", result.err)
		}

		execTime := time.Since(startTime)
		e.logStore.AddLog(functionKey, LevelInfo,
			fmt.Sprintf("Function executed successfully: %s (execution time: %v, response size: %d bytes)",
				entrypoint, execTime, len(result.output)))

		cb.RecordSuccess()
		return result.output, nil

	case <-ctx.Done():
		isOpen := cb.RecordFailure()
		errMsg := fmt.Sprintf("Function execution timed out after %v", e.defaultTimeout)
		e.logStore.AddLog(functionKey, LevelError, errMsg)

		if isOpen {
			cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
			e.logger.Printf(cbMsg)
			e.logStore.AddLog(functionKey, LevelError, cbMsg)
		}

		return nil, fmt.Errorf("%s", errMsg)
	}
}

func (e *Engine) LoadFunction(namespace, name, identifier string, config map[string]string) error {
	e.logger.Printf("Loading function: %s/%s (identifier: %s)", namespace, name, identifier)
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	// Create a copy of the config map
	configCopy := make(map[string]string)
	for k, v := range config {
		configCopy[k] = v
	}

	loadStart := time.Now()
	wasmBytes, versionInfo, err := e.registry.Pull(namespace, name, identifier)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to fetch WASM file from registry: %v", err)
		e.logger.Errorf(errMsg)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return fmt.Errorf("failed to fetch WASM file from registry: %w", err)
	}
	e.logStore.AddLog(functionKey, LevelInfo,
		fmt.Sprintf("Function pulled from registry (size: %d bytes, time: %v)",
			len(wasmBytes), time.Since(loadStart)))

	actualDigest := versionInfo.FullDigest

	// Check if already loaded with same digest and config
	alreadyLoaded := e.pluginManager.IsPluginLoaded(functionKey)

	if alreadyLoaded {
		digestChanged := e.pluginManager.HasDigestChanged(functionKey, actualDigest)
		configChanged := e.pluginManager.HasConfigChanged(functionKey, configCopy)

		if !digestChanged && !configChanged {
			e.logger.Printf("Function %s already loaded with same digest and config", functionKey)
			e.logStore.AddLog(functionKey, LevelInfo, "Function already loaded with same digest and config")
			return nil
		}

		if digestChanged {
			oldDigest, _ := e.pluginManager.GetPluginDigest(functionKey)
			e.logger.Printf("Function %s digest changed from %s to %s, reloading",
				functionKey, oldDigest, actualDigest)
			e.logStore.AddLog(functionKey, LevelInfo,
				fmt.Sprintf("Function digest changed from %s to %s, reloading",
					oldDigest, actualDigest))
		}

		if configChanged {
			e.logger.Printf("Function %s configuration changed, reloading", functionKey)
			e.logStore.AddLog(functionKey, LevelInfo, "Function configuration changed, reloading")
		}

		// Remove the old plugin from the plugin manager
		e.pluginManager.RemovePlugin(functionKey)

		// Remove circuit breaker for this function
		e.circuitBreakers.RemoveCircuitBreaker(functionKey)
	}

	// Create a new plugin instance
	initStart := time.Now()
	plugin, err := components.CreatePlugin(wasmBytes, versionInfo, configCopy)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to initialize plugin: %v", err)
		e.logger.Errorf(errMsg)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}
	e.logStore.AddLog(functionKey, LevelInfo,
		fmt.Sprintf("Plugin initialized successfully (time: %v)", time.Since(initStart)))

	// Store the plugin in the plugin manager
	e.pluginManager.StorePlugin(functionKey, plugin, actualDigest, configCopy)

	successMsg := fmt.Sprintf("Function loaded successfully: %s", functionKey)
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)
	return nil
}

func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error) {
	e.logger.Printf("Building function: %s/%s", namespace, name)

	buildStart := time.Now()

	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		name = filepath.Base(path)
	}

	buildResult, err := e.functionService.BuildFunction(path, config)
	if err != nil {
		e.logger.Errorf("Failed to build function: %v", err)
		return nil, fmt.Errorf("failed to build function: %w", err)
	}

	wasmBytes, err := os.ReadFile(buildResult.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read wasm file: %w", err)
	}

	if tag == "" {
		tag = buildResult.Digest
	}

	if err := e.registry.Push(namespace, name, wasmBytes, buildResult.Digest, tag, config.FunctionSettings.VersionSettings); err != nil {
		return nil, fmt.Errorf("failed to store in registry: %w", err)
	}

	e.logger.Printf("Function built successfully: %s/%s (digest: %s, tag: %s)",
		namespace, name, buildResult.Digest, tag)

	return &types.BuildResult{
		Name:      name,
		Namespace: namespace,
		Digest:    buildResult.Digest,
		BuildTime: time.Since(buildStart),
		Tag:       tag,
	}, nil
}

func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	e.logger.Printf("Reassigning tag %s to digest %s for function: %s/%s", tag, newDigest, namespace, name)

	if err := e.registry.ReassignTag(namespace, name, tag, newDigest); err != nil {
		e.logger.Errorf("Failed to reassign tag: %v", err)
		return fmt.Errorf("failed to reassign tag: %w", err)
	}

	e.logger.Printf("Tag %s reassigned to digest %s for function: %s/%s", tag, newDigest, namespace, name)
	return nil
}

func (e *Engine) UnloadFunction(namespace, name string) error {
	e.logger.Printf("Unloading function: %s/%s", namespace, name)
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, "Unloading function")

	// Check if the function is loaded
	if !e.pluginManager.IsPluginLoaded(functionKey) {
		notLoadedMsg := fmt.Sprintf("Function %s is not loaded, nothing to unload", functionKey)
		e.logger.Printf(notLoadedMsg)
		e.logStore.AddLog(functionKey, LevelInfo, notLoadedMsg)
		return nil
	}

	unloadStart := time.Now()

	// Remove the plugin from the plugin manager
	e.pluginManager.RemovePlugin(functionKey)

	// Remove circuit breaker for this function
	e.circuitBreakers.RemoveCircuitBreaker(functionKey)

	successMsg := fmt.Sprintf("Function %s unloaded successfully (time: %v)", functionKey, time.Since(unloadStart))
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)

	e.logStore.AddLog(functionKey, LevelInfo, "Function unloaded - this is the final log entry")

	return nil
}
