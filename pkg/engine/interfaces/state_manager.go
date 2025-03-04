package interfaces

import (
	"time"
)

// FunctionState contains the complete state information for a function
type FunctionState struct {
	// Basic state
	Loaded           bool
	Running          bool
	Stopped          bool
	PreviouslyLoaded bool

	// Configuration
	Config map[string]string

	// Execution stats
	LastExecutionTime    time.Time
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64

	// Circuit breaker status
	CircuitBreakerOpen bool

	// Registry info
	Digest string
	Tags   []string
}

// StateManager defines the interface for managing function state
type StateManager interface {
	// GetState returns the complete state of a function
	GetState(namespace, name string) FunctionState

	// IsLoaded returns true if the function is currently loaded
	IsLoaded(namespace, name string) bool

	// IsStopped returns true if the function is explicitly stopped
	IsStopped(namespace, name string) bool

	// WasPreviouslyLoaded returns true if the function was previously loaded in this session
	WasPreviouslyLoaded(namespace, name string) (bool, map[string]string)

	// MarkLoaded marks a function as loaded with the given digest and config
	MarkLoaded(namespace, name, digest string, config map[string]string)

	// MarkUnloaded marks a function as unloaded but remembers its configuration
	MarkUnloaded(namespace, name string)

	// MarkStopped marks a function as explicitly stopped
	MarkStopped(namespace, name string)

	// ClearStoppedStatus clears the stopped status of a function
	ClearStoppedStatus(namespace, name string)

	// GetDigest returns the current digest for a function
	GetDigest(namespace, name string) (string, bool)

	// GetRuntime retrieves a runtime for a function
	GetRuntime(namespace, name string) (WasmRuntime, bool)

	// StoreRuntime stores a runtime for a function
	StoreRuntime(namespace, name string, runtime WasmRuntime)

	// RemoveRuntime removes a runtime for a function
	RemoveRuntime(namespace, name string)

	// ListLoaded returns a list of all loaded functions
	ListLoaded() []string
}

// CircuitBreakerManager manages circuit breakers for functions
type CircuitBreakerManager interface {
	// GetCircuitBreaker returns the circuit breaker for a function
	GetCircuitBreaker(namespace, name string) CircuitBreaker

	// RecordSuccess records a successful execution for a function
	RecordSuccess(namespace, name string)

	// RecordFailure records a failed execution for a function
	RecordFailure(namespace, name string) bool

	// Reset resets the circuit breaker for a function
	Reset(namespace, name string)
}

// CircuitBreaker defines the interface for a single function's circuit breaker
type CircuitBreaker interface {
	// IsOpen returns true if the circuit breaker is open (preventing calls)
	IsOpen() bool

	// RecordSuccess records a successful execution
	RecordSuccess()

	// RecordFailure records a failed execution and returns true if the circuit is now open
	RecordFailure() bool

	// Reset resets the circuit breaker state to closed
	Reset()
}
