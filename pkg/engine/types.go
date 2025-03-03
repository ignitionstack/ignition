//go:generate mockgen -source=types.go -destination=mocks/mocks.go -package=mocks

package engine

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionLifecycle defines the core function lifecycle operations
type FunctionLifecycle interface {
	// Load a function
	LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string) error
	
	// Load a function with force option
	LoadFunctionWithForce(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error
	
	// Execute a function
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error)
	
	// Unload a function (can be reloaded)
	UnloadFunction(namespace, name string) error
	
	// Stop a function (prevents auto-reload)
	StopFunction(namespace, name string) error
}

// FunctionState provides function state information
type FunctionState interface {
	// Check if function is currently loaded
	IsLoaded(namespace, name string) bool
	
	// Check if function was previously loaded
	WasPreviouslyLoaded(namespace, name string) (bool, map[string]string)
	
	// Check if function is explicitly stopped
	IsStopped(namespace, name string) bool
}

// FunctionManager handles function operations including building
type FunctionManager interface {
	FunctionLifecycle
	FunctionState
	
	// Build a function
	BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error)
	
	// Tag management
	ReassignTag(namespace, name, tag, newDigest string) error
}

// RegistryOperator provides access to the registry
type RegistryOperator interface {
	// Get access to the registry
	GetRegistry() registry.Registry
}

// ExecutionContext represents a function execution context
type ExecutionContext struct {
	// Context for cancellation and timeout
	Context context.Context
	
	// Function identification
	Namespace string
	Name      string
	
	// Execution details
	Entrypoint string
	Payload    []byte
	
	// Configuration
	Config map[string]string
}

// ExecutionResult represents the result of a function execution
type ExecutionResult struct {
	// Execution output
	Output []byte
	
	// Execution stats
	ExecutionTime time.Duration
	
	// Error, if any
	Error error
}