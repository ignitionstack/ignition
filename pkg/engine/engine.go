package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v4"
	extism "github.com/extism/go-sdk"
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

// CallFunctionWithContext calls a function with a context that can be used for cancellation
func (e *Engine) CallFunctionWithContext(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes", entrypoint, len(payload)))

	// Quick circuit breaker check
	cb := e.circuitBreakers.GetCircuitBreaker(functionKey)
	if cb.IsOpen() {
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", functionKey)
		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Get the plugin
	plugin, ok := e.pluginManager.GetPlugin(functionKey)
	if !ok {
		// Check for a racing condition where the plugin may have been unloaded
		// since our call to IsPluginLoaded
		e.logStore.AddLog(functionKey, logging.LevelError, "Function not loaded")
		return nil, ErrFunctionNotLoaded
	}

	startTime := time.Now()

	// Result channel with buffer to prevent goroutine leaks
	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	// Cancel context for the goroutine if this function returns
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Execute the plugin call in a goroutine
	go func() {
		_, output, err := plugin.Call(entrypoint, payload)

		// Send the result, handling the case where the context is cancelled
		select {
		case resultCh <- struct {
			output []byte
			err    error
		}{output, err}:
			// Result sent successfully
		case <-execCtx.Done():
			// Context was cancelled, nothing to do
		}
	}()

	// Wait for the result or context cancellation
	select {
	case result := <-resultCh:
		if result.err != nil {
			// Record the failure in the circuit breaker
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

		// Record success in metrics and logs
		execTime := time.Since(startTime)
		e.logStore.AddLog(functionKey, LevelInfo,
			fmt.Sprintf("Function executed successfully: %s (execution time: %v, response size: %d bytes)",
				entrypoint, execTime, len(result.output)))

		cb.RecordSuccess()
		return result.output, nil

	case <-ctx.Done():
		// The context deadline was exceeded or cancelled
		isOpen := cb.RecordFailure()

		// Determine the specific error
		var errMsg string
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("Function execution timed out after %v", e.defaultTimeout)
		} else {
			errMsg = "Function execution was cancelled"
		}

		e.logStore.AddLog(functionKey, LevelError, errMsg)

		if isOpen {
			cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
			e.logger.Printf(cbMsg)
			e.logStore.AddLog(functionKey, LevelError, cbMsg)
		}

		return nil, fmt.Errorf("%s", errMsg)
	}
}

// CallFunction calls a function (wrapper for backward compatibility)
func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	// Create a context with the default timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()

	return e.CallFunctionWithContext(ctx, namespace, name, entrypoint, payload)
}

// LoadFunctionWithContext is a context-aware version of LoadFunction
func (e *Engine) LoadFunctionWithContext(ctx context.Context, namespace, name, identifier string, config map[string]string) error {
	return e.LoadFunctionWithContextAndForce(ctx, namespace, name, identifier, config, false)
}

// LoadFunctionWithContextAndForce is a context-aware version of LoadFunction with optional force loading
func (e *Engine) LoadFunctionWithContextAndForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error {
	e.logger.Printf("Loading function: %s/%s (identifier: %s, force: %v)", namespace, name, identifier, force)
	functionKey := components.GetFunctionKey(namespace, name)

	// Check if the function is stopped - only allow loading if force is true
	if e.IsFunctionStopped(namespace, name) && !force {
		e.logger.Printf("Function %s/%s is stopped and cannot be loaded without force option", namespace, name)
		e.logStore.AddLog(functionKey, LevelError, "Cannot load stopped function. Use 'ignition function run' to explicitly load it")
		return fmt.Errorf("function was explicitly stopped - use 'ignition function run' to load it")
	}

	// If force is true and function is stopped, clear the stopped status
	if force && e.IsFunctionStopped(namespace, name) {
		e.logger.Printf("Force loading stopped function %s/%s - clearing stopped status", namespace, name)
		e.logStore.AddLog(functionKey, LevelInfo, "Force loading stopped function - clearing stopped status")
		e.pluginManager.ClearStoppedStatus(functionKey)
	}

	e.logStore.AddLog(functionKey, LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	// Create a copy of the config map
	configCopy := make(map[string]string)
	for k, v := range config {
		configCopy[k] = v
	}

	// Fetch the WASM bytes from the registry
	loadStart := time.Now()

	// Create a channel for the result
	type pullResult struct {
		wasmBytes   []byte
		versionInfo *registry.VersionInfo
		err         error
	}

	// Use a channel with buffer size 1 to prevent goroutine leaks
	resultCh := make(chan pullResult, 1)

	// Pull in a separate goroutine to allow for cancellation
	go func() {
		wasmBytes, versionInfo, err := e.registry.Pull(namespace, name, identifier)
		select {
		case resultCh <- pullResult{wasmBytes, versionInfo, err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context was cancelled, but we need to send the result to avoid goroutine leak
			select {
			case resultCh <- pullResult{nil, nil, ctx.Err()}:
			default:
				// Channel is already closed or full, nothing to do
			}
		}
	}()

	// Wait for the result or context cancellation
	var result pullResult
	select {
	case result = <-resultCh:
		// Result received
	case <-ctx.Done():
		// Context cancelled, wait for the result to avoid goroutine leak
		result = <-resultCh
	}

	// Check for errors from the Pull operation
	if result.err != nil {
		errMsg := fmt.Sprintf("Failed to fetch WASM file from registry: %v", result.err)
		e.logger.Errorf(errMsg)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return fmt.Errorf("failed to fetch WASM file from registry: %w", result.err)
	}

	wasmBytes, versionInfo := result.wasmBytes, result.versionInfo

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

	// Create plugin in a cancellable goroutine
	type pluginResult struct {
		plugin *extism.Plugin
		err    error
	}

	pluginCh := make(chan pluginResult, 1)

	go func() {
		plugin, err := components.CreatePlugin(wasmBytes, versionInfo, configCopy)
		select {
		case pluginCh <- pluginResult{plugin, err}:
			// Result sent successfully
		case <-ctx.Done():
			// Context was cancelled, but cleanup and send result to avoid goroutine leak
			if plugin != nil && err == nil {
				plugin.Close(context.Background())
			}
			select {
			case pluginCh <- pluginResult{nil, ctx.Err()}:
			default:
				// Channel is already closed or full, nothing to do
			}
		}
	}()

	// Wait for plugin creation or context cancellation
	var pluginRes pluginResult
	select {
	case pluginRes = <-pluginCh:
		// Result received
	case <-ctx.Done():
		// Context cancelled, wait for the result to avoid goroutine leak
		pluginRes = <-pluginCh
	}

	// Check for errors from the plugin creation
	if pluginRes.err != nil {
		errMsg := fmt.Sprintf("Failed to initialize plugin: %v", pluginRes.err)
		e.logger.Errorf(errMsg)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return fmt.Errorf("failed to initialize plugin: %w", pluginRes.err)
	}

	plugin := pluginRes.plugin

	e.logStore.AddLog(functionKey, LevelInfo,
		fmt.Sprintf("Plugin initialized successfully (time: %v)", time.Since(initStart)))

	// Store the plugin in the plugin manager
	e.pluginManager.StorePlugin(functionKey, plugin, actualDigest, configCopy)

	successMsg := fmt.Sprintf("Function loaded successfully: %s", functionKey)
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)
	return nil
}

// LoadFunction loads a function into memory (wrapper for backward compatibility)
func (e *Engine) LoadFunction(namespace, name, identifier string, config map[string]string) error {
	// Create a context with the default timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()

	return e.LoadFunctionWithContextAndForce(ctx, namespace, name, identifier, config, false)
}

func (e *Engine) LoadFunctionWithForce(namespace, name, identifier string, config map[string]string, force bool) error {
	// Create a context with the default timeout
	ctx, cancel := context.WithTimeout(context.Background(), e.defaultTimeout)
	defer cancel()

	return e.LoadFunctionWithContextAndForce(ctx, namespace, name, identifier, config, force)
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

// StopFunction fully stops a function and prevents automatic reloading
func (e *Engine) StopFunction(namespace, name string) error {
	e.logger.Printf("Stopping function: %s/%s", namespace, name)
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, "Stopping function")

	// Check if the function is already stopped
	if e.pluginManager.IsFunctionStopped(functionKey) {
		alreadyStoppedMsg := fmt.Sprintf("Function %s is already stopped", functionKey)
		e.logger.Printf(alreadyStoppedMsg)
		e.logStore.AddLog(functionKey, LevelInfo, alreadyStoppedMsg)
		return nil
	}

	stopStart := time.Now()

	// Stop the function using the plugin manager's StopFunction method
	e.pluginManager.StopFunction(functionKey)

	// Remove circuit breaker for this function
	e.circuitBreakers.RemoveCircuitBreaker(functionKey)

	successMsg := fmt.Sprintf("Function %s stopped successfully (time: %v)", functionKey, time.Since(stopStart))
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)

	e.logStore.AddLog(functionKey, LevelInfo, "Function stopped - will not be automatically reloaded")

	return nil
}

// IsFunctionStopped checks if a function has been explicitly stopped
func (e *Engine) IsFunctionStopped(namespace, name string) bool {
	functionKey := components.GetFunctionKey(namespace, name)
	return e.pluginManager.IsFunctionStopped(functionKey)
}
