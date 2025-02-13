package engine

import (
	"context"
	"fmt"
	"log"
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

type Engine struct {
	reg        registry.Registry
	plugins    map[string]*extism.Plugin
	pluginsMux sync.RWMutex
	socketPath string
	httpAddr   string
	logger     Logger
}

func NewEngine(socketPath, httpAddr string, registryDir string) (*Engine, error) {

	opts := badger.DefaultOptions(filepath.Join(registryDir, "registry.db"))
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry database: %w", err)
	}

	reg := localRegistry.NewLocalRegistry(registryDir, db)
	logger := &stdLogger{log.New(os.Stdout, "", log.LstdFlags)}

	return &Engine{
		reg:        reg,
		plugins:    make(map[string]*extism.Plugin),
		socketPath: socketPath,
		httpAddr:   httpAddr,
		logger:     logger,
	}, nil
}

func (e *Engine) Start() error {
	handlers := NewHandlers(e, e.logger)
	server := NewServer(e.socketPath, e.httpAddr, handlers, e.logger)

	return server.Start()
}

func (e *Engine) CallFunction(namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionName := fmt.Sprintf("%s/%s", namespace, name)

	e.pluginsMux.RLock()
	plugin, ok := e.plugins[functionName]
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

func (e *Engine) LoadFunction(namespace, name, identifier string) error {
	e.logger.Printf("Loading function: %s/%s (identifier: %s)", namespace, name, identifier)

	var wasmBytes []byte
	var err error

	wasmBytes, _, err = e.reg.Pull(namespace, name, identifier)
	if err != nil {
		e.logger.Errorf("Failed to fetch WASM file from registry: %v", err)
		return fmt.Errorf("failed to fetch WASM file from registry: %w", err)
	}

	// Create the Extism manifest
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
	}

	// Initialize the plugin
	plugin, err := extism.NewPlugin(context.Background(), manifest, extism.PluginConfig{
		EnableWasi: true,
	}, []extism.HostFunction{})
	if err != nil {
		e.logger.Errorf("Failed to initialize plugin: %v", err)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Store the plugin
	e.pluginsMux.Lock()
	defer e.pluginsMux.Unlock()
	key := fmt.Sprintf("%s/%s", namespace, name)
	e.plugins[key] = plugin

	e.logger.Printf("Function loaded successfully: %s", key)
	return nil
}

func (e *Engine) BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*BuildResult, error) {
	e.logger.Printf("Building function: %s/%s", namespace, name)

	buildStart := time.Now()

	service := services.NewFunctionService()

	// Build the function
	buildResult, err := service.BuildFunction(path, config)
	if err != nil {
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

	// If namespace or name is empty, use defaults
	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		name = filepath.Base(path) // Use the directory name as the function name
	}

	// Store in registry
	if err := e.reg.Push(namespace, name, wasmBytes, buildResult.Digest, tag); err != nil {
		return nil, fmt.Errorf("failed to store in registry: %w", err)
	}

	e.logger.Printf("Function built successfully: %s/%s (digest: %s, tag: %s)",
		namespace, name, buildResult.Digest, tag)

	resp := &BuildResult{
		Name:      name,
		Namespace: namespace,
		Digest:    buildResult.Digest,
		BuildTime: time.Since(buildStart),
		Tag:       tag,
	}

	return resp, nil
}

func (e *Engine) ReassignTag(namespace, name, tag, newDigest string) error {
	e.logger.Printf("Reassigning tag %s to digest %s for function: %s/%s", tag, newDigest, namespace, name)

	// Reassign the tag in the registry
	if err := e.reg.ReassignTag(namespace, name, tag, newDigest); err != nil {
		e.logger.Errorf("Failed to reassign tag: %v", err)
		return fmt.Errorf("failed to reassign tag: %w", err)
	}

	e.logger.Printf("Tag %s reassigned to digest %s for function: %s/%s", tag, newDigest, namespace, name)
	return nil
}
