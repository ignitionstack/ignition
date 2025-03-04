package wasm

import (
	"context"
	"fmt"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
	"github.com/ignitionstack/ignition/pkg/registry"
)

// ExtismRuntime implements the interfaces.WasmRuntime interface using Extism
type ExtismRuntime struct {
	plugin *extism.Plugin
	info   interfaces.RuntimeInfo
}

// NewExtismRuntime creates a new ExtismRuntime
func NewExtismRuntime(plugin *extism.Plugin, wasmSize int, digest string, config map[string]string) *ExtismRuntime {
	return &ExtismRuntime{
		plugin: plugin,
		info: interfaces.RuntimeInfo{
			Size:     wasmSize,
			Digest:   digest,
			Config:   config,
			Manifest: make(map[string]interface{}),
		},
	}
}

// ExecuteFunction implements interfaces.WasmRuntime
func (r *ExtismRuntime) ExecuteFunction(ctx context.Context, entrypoint string, payload []byte) ([]byte, error) {
	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	// Execute in a goroutine to allow for cancellation
	go func() {
		code, output, err := r.plugin.Call(entrypoint, payload)
		if err != nil {
			resultCh <- struct {
				output []byte
				err    error
			}{nil, err}
			return
		}

		if code != 0 {
			resultCh <- struct {
				output []byte
				err    error
			}{nil, fmt.Errorf("function returned non-zero exit code: %d", code)}
			return
		}

		resultCh <- struct {
			output []byte
			err    error
		}{output, nil}
	}()

	// Wait for result or context cancellation
	select {
	case result := <-resultCh:
		return result.output, result.err
	case <-ctx.Done():
		// Context was cancelled
		return nil, ctx.Err()
	}
}

// Close implements interfaces.WasmRuntime
func (r *ExtismRuntime) Close(ctx context.Context) error {
	// Extism's Close doesn't support context, so we'll ignore it for now
	r.plugin.Close(context.Background())
	return nil
}

// GetInfo implements interfaces.WasmRuntime
func (r *ExtismRuntime) GetInfo() interfaces.RuntimeInfo {
	return r.info
}

// ExtismRuntimeFactory implements interfaces.WasmRuntimeFactory
type ExtismRuntimeFactory struct{}

// CreateRuntime implements interfaces.WasmRuntimeFactory
func (f *ExtismRuntimeFactory) CreateRuntime(ctx context.Context, wasmBytes []byte, config map[string]string) (interfaces.WasmRuntime, error) {
	// Convert config map to extism manifest
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Config: convertConfigToExtism(config),
	}

	// Create the plugin
	pluginConfig := extism.PluginConfig{
		EnableWasi: true, // Default to WASI enabled
	}

	plugin, err := extism.NewPlugin(ctx, manifest, pluginConfig, []extism.HostFunction{})
	if err != nil {
		return nil, fmt.Errorf("failed to create extism plugin: %w", err)
	}

	// Create our runtime wrapper
	runtime := NewExtismRuntime(
		plugin,
		len(wasmBytes),
		"", // We don't have digest here, it will be set by the caller
		config,
	)

	return runtime, nil
}

// Helper function to convert our config map to extism config format
func convertConfigToExtism(config map[string]string) map[string]string {
	// For now, we just pass through the config as-is
	return config
}

// CreateExtismRuntimeFromVersionInfo creates an ExtismRuntime from registry info
func CreateExtismRuntimeFromVersionInfo(wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (interfaces.WasmRuntime, error) {
	// Create the plugin using extism's API
	manifest := extism.Manifest{
		AllowedHosts: versionInfo.Settings.AllowedUrls,
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Config: convertConfigToExtism(config),
	}

	pluginConfig := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	plugin, err := extism.NewPlugin(context.Background(), manifest, pluginConfig, []extism.HostFunction{})
	if err != nil {
		return nil, fmt.Errorf("failed to create extism plugin: %w", err)
	}

	// Create our runtime wrapper
	runtime := NewExtismRuntime(
		plugin,
		len(wasmBytes),
		versionInfo.FullDigest,
		config,
	)

	return runtime, nil
}
