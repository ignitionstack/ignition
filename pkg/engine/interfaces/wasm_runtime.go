package interfaces

import "context"

// WasmRuntime defines the interface for a WebAssembly runtime implementation
// This allows us to abstract the underlying WebAssembly runtime (currently extism)
type WasmRuntime interface {
	// ExecuteFunction calls a function in the WebAssembly module
	ExecuteFunction(ctx context.Context, entrypoint string, payload []byte) ([]byte, error)

	// Close frees resources associated with this runtime instance
	Close(ctx context.Context) error

	// GetInfo returns metadata about the runtime instance
	GetInfo() RuntimeInfo
}

// RuntimeInfo contains metadata about a WebAssembly runtime instance
type RuntimeInfo struct {
	// Size in bytes of the WebAssembly module
	Size int

	// Digest of the WebAssembly module
	Digest string

	// Manifest settings from the function manifest
	Manifest map[string]interface{}

	// Config values passed when loading the function
	Config map[string]string
}

// WasmRuntimeFactory creates WasmRuntime instances
type WasmRuntimeFactory interface {
	// CreateRuntime creates a new WasmRuntime instance from WebAssembly bytes
	CreateRuntime(ctx context.Context, wasmBytes []byte, config map[string]string) (WasmRuntime, error)
}
