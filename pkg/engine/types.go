//go:generate mockgen -source=types.go -destination=mocks/mocks.go -package=mocks

package engine

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionManager defines all function operations supported by the engine.
type FunctionManager interface {
	// Core function operations
	LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error)
	UnloadFunction(namespace, name string) error
	StopFunction(namespace, name string) error
	
	// Function state queries
	GetFunctionState(namespace, name string) FunctionState
	
	// Function building operations
	BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error)
	ReassignTag(namespace, name, tag, newDigest string) error
}

// FunctionState contains the complete state information for a function
type FunctionState struct {
	Loaded           bool
	Stopped          bool
	PreviouslyLoaded bool
	Config           map[string]string
}

// Type aliases for component interfaces.
type PluginManager = components.PluginManager
type PluginManagerSettings = components.PluginManagerSettings
type CircuitBreaker = components.CircuitBreaker
type CircuitBreakerManager = components.CircuitBreakerManager

// ExecutionContext represents a function execution context.
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

// ExecutionResult represents the result of a function execution.
type ExecutionResult struct {
	// Execution output
	Output []byte

	// Execution stats
	ExecutionTime time.Duration

	// Error, if any
	Error error
}
