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
	"github.com/ignitionstack/ignition/pkg/types"
)

type CircuitBreaker struct {
	failures         int
	lastFailure      time.Time
	state            string
	failureThreshold int
	resetTimeout     time.Duration
	mutex            sync.RWMutex
}

type Engine struct {
	registry        registry.Registry
	functionService services.FunctionService
	plugins         map[string]*extism.Plugin
	pluginsMux      sync.RWMutex
	socketPath      string
	httpAddr        string
	logger          Logger
	initialized     bool

	pluginLastUsed map[string]time.Time
	ttlDuration    time.Duration
	cleanupTicker  *time.Ticker

	defaultTimeout  time.Duration
	circuitBreakers map[string]*CircuitBreaker
	cbMux           sync.RWMutex

	logStore *FunctionLogStore
}

func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {
	logger := NewStdLogger(os.Stdout)

	return NewEngineWithLogger(socketPath, httpAddr, registryDir, logger)
}

func NewEngineWithLogger(socketPath, httpAddr string, registryDir string, logger Logger) (*Engine, error) {
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

		pluginLastUsed:  make(map[string]time.Time),
		ttlDuration:     30 * time.Minute,
		defaultTimeout:  30 * time.Second,
		circuitBreakers: make(map[string]*CircuitBreaker),
		logStore:        NewFunctionLogStore(1000),
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

	defer func() {
		e.logger.Printf("Engine Start function exiting, stopping cleanup goroutine")
		if e.cleanupTicker != nil {
			e.cleanupTicker.Stop()
		}
		cancel()
	}()

	e.cleanupTicker = time.NewTicker(5 * time.Minute)
	go e.cleanupUnusedPlugins(ctx)

	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	return server.Start()
}

func (e *Engine) cleanupUnusedPlugins(ctx context.Context) {
	e.logger.Printf("Starting plugin cleanup goroutine")

	for {
		select {
		case <-e.cleanupTicker.C:
			e.logger.Printf("Running plugin cleanup")
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

		case <-ctx.Done():
			e.logger.Printf("Cleanup goroutine received shutdown signal")
			return
		}
	}
}

func getFunctionKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func (e *Engine) IsLoaded(namespace, name string) bool {
	functionKey := getFunctionKey(namespace, name)

	e.pluginsMux.RLock()
	_, exists := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	return exists
}

func (e *Engine) GetRegistry() registry.Registry {
	return e.registry
}

func newCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		failures:         0,
		state:            "closed",
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == "half-open" {
		cb.failures = 0
		cb.state = "closed"
	}
}

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

func (cb *CircuitBreaker) isOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if cb.state == "open" {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
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

func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := getFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes", entrypoint, len(payload)))

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
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", functionKey)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	e.pluginsMux.RLock()
	plugin, ok := e.plugins[functionKey]
	if ok {
		e.pluginLastUsed[functionKey] = time.Now()
	} else {
		e.pluginsMux.RUnlock()
		e.logStore.AddLog(functionKey, LevelError, "Function not loaded")
		return nil, ErrFunctionNotLoaded
	}
	e.pluginsMux.RUnlock()

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
			isOpen := cb.recordFailure()
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

		cb.recordSuccess()
		return result.output, nil

	case <-ctx.Done():
		isOpen := cb.recordFailure()
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
	functionKey := getFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, fmt.Sprintf("Loading function with identifier: %s", identifier))

	e.pluginsMux.RLock()
	_, alreadyLoaded := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	if alreadyLoaded {
		e.logger.Printf("Function %s already loaded", functionKey)
		e.logStore.AddLog(functionKey, LevelInfo, "Function already loaded")

		e.pluginsMux.Lock()
		e.pluginLastUsed[functionKey] = time.Now()
		e.pluginsMux.Unlock()

		return nil
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

	initStart := time.Now()
	plugin, err := createPlugin(wasmBytes, versionInfo, nil) // Pass nil for config as it will be added in the handler
	if err != nil {
		errMsg := fmt.Sprintf("Failed to initialize plugin: %v", err)
		e.logger.Errorf(errMsg)
		e.logStore.AddLog(functionKey, LevelError, errMsg)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}
	e.logStore.AddLog(functionKey, LevelInfo,
		fmt.Sprintf("Plugin initialized successfully (time: %v)", time.Since(initStart)))

	e.pluginsMux.Lock()
	defer e.pluginsMux.Unlock()

	if _, exists := e.plugins[functionKey]; exists {
		plugin.Close(context.TODO())
		e.logStore.AddLog(functionKey, LevelInfo, "Function already loaded by another request")
		return nil
	}

	e.plugins[functionKey] = plugin
	e.pluginLastUsed[functionKey] = time.Now()

	e.cbMux.Lock()
	if _, exists := e.circuitBreakers[functionKey]; !exists {
		e.circuitBreakers[functionKey] = newCircuitBreaker()
	}
	e.cbMux.Unlock()

	successMsg := fmt.Sprintf("Function loaded successfully: %s", functionKey)
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)
	return nil
}

func createPlugin(wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (*extism.Plugin, error) {
	manifest := extism.Manifest{
		AllowedHosts: versionInfo.Settings.AllowedUrls,
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Config: config,
	}

	pluginConfig := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	return extism.NewPlugin(context.Background(), manifest, pluginConfig, []extism.HostFunction{})
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
	functionKey := getFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, LevelInfo, "Unloading function")

	e.pluginsMux.RLock()
	plugin, exists := e.plugins[functionKey]
	e.pluginsMux.RUnlock()

	if !exists {
		notLoadedMsg := fmt.Sprintf("Function %s is not loaded, nothing to unload", functionKey)
		e.logger.Printf(notLoadedMsg)
		e.logStore.AddLog(functionKey, LevelInfo, notLoadedMsg)
		return nil
	}

	e.pluginsMux.Lock()
	defer e.pluginsMux.Unlock()

	unloadStart := time.Now()

	plugin.Close(context.TODO())
	delete(e.plugins, functionKey)
	delete(e.pluginLastUsed, functionKey)

	e.cbMux.Lock()
	delete(e.circuitBreakers, functionKey)
	e.cbMux.Unlock()

	successMsg := fmt.Sprintf("Function %s unloaded successfully (time: %v)", functionKey, time.Since(unloadStart))
	e.logger.Printf(successMsg)
	e.logStore.AddLog(functionKey, LevelInfo, successMsg)

	e.logStore.AddLog(functionKey, LevelInfo, "Function unloaded - this is the final log entry")

	return nil
}
