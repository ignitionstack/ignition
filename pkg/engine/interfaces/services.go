package interfaces

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// FunctionService defines the high-level interface for function operations
type FunctionService interface {
	// LoadFunction loads a function into memory
	LoadFunction(ctx context.Context, namespace, name, identifier string, config map[string]string, force bool) error

	// UnloadFunction unloads a function from memory
	UnloadFunction(namespace, name string) error

	// StopFunction stops a function and prevents auto-loading
	StopFunction(namespace, name string) error

	// CallFunction calls a function with the given parameters
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error)

	// GetFunctionState returns the current state of a function
	GetFunctionState(namespace, name string) FunctionState

	// BuildFunction builds a function from source
	BuildFunction(namespace, name, path, tag string, config manifest.FunctionManifest) (*types.BuildResult, error)

	// ReassignTag changes a tag to point to a different digest
	ReassignTag(namespace, name, tag, newDigest string) error
}

// RegistryService defines interactions with the function registry
type RegistryService interface {
	// Pull fetches a function from the registry
	Pull(ctx context.Context, namespace, name, identifier string) ([]byte, string, error)

	// Push uploads a function to the registry
	Push(ctx context.Context, namespace, name string, wasmBytes []byte, digest, tag string, settings manifest.FunctionVersionSettings) error

	// ListFunctions returns a list of functions in the registry
	ListFunctions(ctx context.Context) ([]types.FunctionInfo, error)

	// GetFunction returns information about a function
	GetFunction(ctx context.Context, namespace, name string) (*types.FunctionInfo, error)
}

// ExecutionService defines the interface for executing functions
type ExecutionService interface {
	// Execute calls a function with execution context
	Execute(ctx context.Context, params ExecutionParams) (ExecutionResult, error)

	// GetStats returns execution statistics for a function
	GetStats(namespace, name string) ExecutionStats
}

// ExecutionParams contains parameters for function execution
type ExecutionParams struct {
	Namespace   string
	Name        string
	Entrypoint  string
	Payload     []byte
	Timeout     time.Duration
	RetryPolicy RetryPolicy
}

// ExecutionResult contains the result of a function execution
type ExecutionResult struct {
	Output        []byte
	ExecutionTime time.Duration
	MemoryUsage   int64
	CPUTime       time.Duration
	Error         error
}

// ExecutionStats contains execution statistics
type ExecutionStats struct {
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	AverageExecutionTime time.Duration
	MaxExecutionTime     time.Duration
	LastExecutionTime    time.Time
}

// RetryPolicy defines how to retry failed executions
type RetryPolicy struct {
	MaxRetries  int
	BackoffBase time.Duration
	MaxBackoff  time.Duration
}
