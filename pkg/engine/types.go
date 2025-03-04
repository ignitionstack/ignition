//go:generate mockgen -source=types.go -destination=mocks/mocks.go -package=mocks

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionManager defines all function operations supported by the engine.
// It's split into logical operation groups for better organization.
type FunctionManager interface {
	// Runtime operations - core function lifecycle management
	RuntimeOperations
	
	// Build operations - function building and registry management 
	BuildOperations
	
	// State operations - function state queries
	StateOperations
}

// RuntimeOperations defines the core runtime operations for functions.
type RuntimeOperations interface {
	// LoadFunction loads a function with the specified identifier and configuration.
	// If force is true, it will load even if the function is marked as stopped.
	LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error
	
	// CallFunction calls a function with the specified parameters.
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error)
	
	// UnloadFunction unloads a function, removing it from memory.
	UnloadFunction(namespace, name string) error
	
	// StopFunction stops a function and marks it as stopped so it won't be auto-loaded.
	StopFunction(namespace, name string) error
}

// BuildOperations defines operations for building and tagging functions.
type BuildOperations interface {
	// BuildFunction builds a function from source and adds it to the registry.
	BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error)
	
	// ReassignTag changes a tag to point to a different digest.
	ReassignTag(namespace, name, tag, newDigest string) error
}

// StateOperations defines operations for querying function state.
type StateOperations interface {
	// GetFunctionState returns the current state of a function.
	GetFunctionState(namespace, name string) FunctionState
}

// FunctionState contains the complete state information for a function.
// It provides a unified view of a function's current status.
type FunctionState struct {
	// Basic state
	Loaded           bool              // Whether the function is currently loaded
	Running          bool              // Whether the function is currently running
	Stopped          bool              // Whether the function has been explicitly stopped
	PreviouslyLoaded bool              // Whether the function was previously loaded in this session
	
	// Configuration
	Config           map[string]string // Current function configuration
	
	// Execution stats
	LastExecutionTime    time.Time     // When the function was last executed
	TotalExecutions      int64         // Total number of executions
	SuccessfulExecutions int64         // Number of successful executions
	FailedExecutions     int64         // Number of failed executions
	
	// Circuit breaker status
	CircuitBreakerOpen bool              // Whether the circuit breaker is open
	
	// Registry info
	Digest           string              // Current function digest
	Tags             []string            // Tags associated with this function
}

// GetFunctionKey returns a unique key for a function based on namespace and name.
func GetFunctionKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
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
