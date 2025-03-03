package resource

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Logger interface for logging.
type Logger interface {
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// Default logger.
var logger Logger = log.New(log.Writer(), "resource: ", log.LstdFlags)

// MemoryLimit represents memory limits in bytes.
type MemoryLimit int64

// Common memory size constants.
const (
	Kilobyte MemoryLimit = 1024
	Megabyte             = Kilobyte * 1024
	Gigabyte             = Megabyte * 1024
)

// Limits defines execution resource limits for functions.
type Limits struct {
	// Memory limits
	MemoryLimit MemoryLimit // Maximum memory for a plugin instance

	// Execution limits
	MaxExecutionTime    time.Duration // Maximum execution time for a single call
	MaxConcurrentCalls  int           // Maximum concurrent calls across all functions
	MaxCallsPerFunction int           // Maximum concurrent calls to a single function

	// Plugin limits
	MaxPluginsLoaded      int           // Maximum number of plugins that can be loaded simultaneously
	PluginIdleTimeout     time.Duration // How long plugins can remain idle before unloading
	PluginCleanupInterval time.Duration // How often to check for idle plugins
}

// DefaultLimits returns a set of reasonable default resource limits.
func DefaultLimits() Limits {
	return Limits{
		// Memory limits
		MemoryLimit: 128 * Megabyte,

		// Execution limits
		MaxExecutionTime:    30 * time.Second,
		MaxConcurrentCalls:  100,
		MaxCallsPerFunction: 10,

		// Plugin limits
		MaxPluginsLoaded:      50,
		PluginIdleTimeout:     10 * time.Minute,
		PluginCleanupInterval: 1 * time.Minute,
	}
}

// Manager handles resource allocation and limiting.
type Manager struct {
	limits        Limits
	executionSem  chan struct{}            // Semaphore for limiting concurrent executions
	functionSems  map[string]chan struct{} // Per-function semaphores
	functionSemMu sync.Mutex               // Mutex for the functionSems map
}

// NewManager creates a new resource manager with the specified limits.
func NewManager(limits Limits) *Manager {
	return &Manager{
		limits:       limits,
		executionSem: make(chan struct{}, limits.MaxConcurrentCalls),
		functionSems: make(map[string]chan struct{}),
	}
}

// Returns true if successful, false if the maximum concurrent calls limit was reached.
func (rm *Manager) AcquireExecution(ctx context.Context) error {
	select {
	case rm.executionSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to acquire execution slot: %w", ctx.Err())
	}
}

// ReleaseExecution releases a global execution slot.
func (rm *Manager) ReleaseExecution() {
	select {
	case <-rm.executionSem:
	default:
		// This should never happen in correct usage
		logger.Println("WARNING: Attempted to release an execution slot that wasn't acquired")
	}
}

// AcquireFunctionExecution attempts to acquire a function-specific execution slot.
func (rm *Manager) AcquireFunctionExecution(ctx context.Context, functionKey string) error {
	// Get or create the function semaphore
	sem := rm.getFunctionSemaphore(functionKey)

	// Try to acquire the function semaphore
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to acquire function execution slot: %w", ctx.Err())
	}
}

// ReleaseFunctionExecution releases a function-specific execution slot.
func (rm *Manager) ReleaseFunctionExecution(functionKey string) {
	rm.functionSemMu.Lock()
	sem, exists := rm.functionSems[functionKey]
	rm.functionSemMu.Unlock()

	if !exists {
		// This should never happen in correct usage
		logger.Printf("WARNING: Attempted to release a function execution slot for unknown function: %s", functionKey)
		return
	}

	select {
	case <-sem:
	default:
		// This should never happen in correct usage
		logger.Printf("WARNING: Attempted to release a function execution slot that wasn't acquired: %s", functionKey)
	}
}

// getFunctionSemaphore gets or creates a semaphore for a function.
func (rm *Manager) getFunctionSemaphore(functionKey string) chan struct{} {
	rm.functionSemMu.Lock()
	defer rm.functionSemMu.Unlock()

	sem, exists := rm.functionSems[functionKey]
	if !exists {
		sem = make(chan struct{}, rm.limits.MaxCallsPerFunction)
		rm.functionSems[functionKey] = sem
	}

	return sem
}

// Acquires both global and function-specific execution slots.
func (rm *Manager) WithResourceLimits(ctx context.Context, functionKey string, operation func() (interface{}, error)) (interface{}, error) {
	// Acquire global execution slot
	if err := rm.AcquireExecution(ctx); err != nil {
		return nil, err
	}
	defer rm.ReleaseExecution()

	// Acquire function-specific execution slot
	if err := rm.AcquireFunctionExecution(ctx, functionKey); err != nil {
		return nil, err
	}
	defer rm.ReleaseFunctionExecution(functionKey)

	// Execute the operation with a timeout
	ctx, cancel := context.WithTimeout(ctx, rm.limits.MaxExecutionTime)
	defer cancel()

	// Create a channel for the result
	type result struct {
		value interface{}
		err   error
	}
	resultCh := make(chan result, 1)

	// Execute the operation in a goroutine
	go func() {
		value, err := operation()
		select {
		case resultCh <- result{value, err}:
		case <-ctx.Done():
		}
	}()

	// Wait for the result or timeout
	select {
	case res := <-resultCh:
		return res.value, res.err
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("operation timed out after %v", rm.limits.MaxExecutionTime)
		}
		return nil, ctx.Err()
	}
}
