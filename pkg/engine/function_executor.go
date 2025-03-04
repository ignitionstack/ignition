package engine

import (
	"context"
	"fmt"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/engine/utils"
)

type FunctionExecutor struct {
	pluginManager   PluginManager
	circuitBreakers CircuitBreakerManager
	logStore        *logging.FunctionLogStore
	logger          logging.Logger
	defaultTimeout  time.Duration
}

func NewFunctionExecutor(pluginManager PluginManager, circuitBreakers CircuitBreakerManager,
	logStore *logging.FunctionLogStore, logger logging.Logger, defaultTimeout time.Duration) *FunctionExecutor {
	return &FunctionExecutor{
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakers,
		logStore:        logStore,
		logger:          logger,
		defaultTimeout:  defaultTimeout,
	}
}

// CallFunction calls a function with the specified parameters.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - namespace: The function namespace
//   - name: The function name
//   - entrypoint: The entry point function to call
//   - payload: The input payload for the function
//
// Returns:
//   - []byte: The output from the function call
//   - error: Any error that occurred during execution
func (e *FunctionExecutor) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := GetFunctionKey(namespace, name)

	// Log the function call
	e.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes", entrypoint, len(payload)))

	// Check the circuit breaker state and get the plugin
	cb, plugin, err := e.prepareExecution(functionKey)
	if err != nil {
		return nil, err
	}

	// Execute the function
	return e.executeFunction(ctx, functionKey, plugin, cb, entrypoint, payload)
}

// prepareExecution checks circuit breaker state and retrieves the plugin.
func (e *FunctionExecutor) prepareExecution(functionKey string) (CircuitBreaker, *extism.Plugin, error) {
	// Check circuit breaker
	cb := e.circuitBreakers.GetCircuitBreaker(functionKey)
	if cb.IsOpen() {
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", functionKey)
		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return nil, nil, WrapEngineError(errMsg, nil)
	}

	// Get the plugin
	plugin, ok := e.pluginManager.GetPlugin(functionKey)
	if !ok {
		e.logStore.AddLog(functionKey, logging.LevelError, "Function not loaded")
		return nil, nil, ErrFunctionNotLoaded
	}

	return cb, plugin, nil
}

type callResult struct {
	output []byte
	err    error
}

// executeFunction performs the actual function execution with proper error handling.
func (e *FunctionExecutor) executeFunction(
	ctx context.Context,
	functionKey string,
	plugin *extism.Plugin,
	cb CircuitBreaker,
	entrypoint string,
	payload []byte,
) ([]byte, error) {
	startTime := time.Now()
	
	// Create a wrapper function for the shared utility
	wrapper := func() (callResult, error) {
		_, output, callErr := plugin.Call(entrypoint, payload)
		return callResult{output, callErr}, nil
	}
	
	// Execute with context cancellation handling
	result, _ := utils.ExecuteWithContext(ctx, wrapper)
	
	// If the context was cancelled, handle it specially
	if ctx.Err() != nil {
		return e.handleCancellation(ctx, functionKey, cb)
	}
	
	// Otherwise process the result with the actual call result
	return e.processResult(functionKey, cb, entrypoint, result, startTime)
}

// logCircuitBreakerOpen logs when a circuit breaker opens
func (e *FunctionExecutor) logCircuitBreakerOpen(functionKey string) {
	cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
	e.logger.Printf(cbMsg)
	e.logStore.AddLog(functionKey, logging.LevelError, cbMsg)
}

// logAndWrapError logs an error message and wraps it as an engine error
func (e *FunctionExecutor) logAndWrapError(functionKey, operation string, err error) error {
	errMsg := fmt.Sprintf("%s: %v", operation, err)
	e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
	return WrapEngineError(operation, err)
}

// GetCircuitBreaker returns the circuit breaker for a function
func (e *FunctionExecutor) GetCircuitBreaker(namespace, name string) CircuitBreaker {
	functionKey := GetFunctionKey(namespace, name)
	return e.circuitBreakers.GetCircuitBreaker(functionKey)
}

// ExecutionStats contains statistics about function execution
type ExecutionStats struct {
	LastExecution        time.Time
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
}

// GetStats returns execution statistics for a function 
func (e *FunctionExecutor) GetStats(namespace, name string) ExecutionStats {
	// Currently we don't track these stats, so return empty values
	// This is a placeholder for future implementation
	return ExecutionStats{
		LastExecution:        time.Time{},
		TotalExecutions:      0,
		SuccessfulExecutions: 0,
		FailedExecutions:     0,
	}
}

// processResult handles both success and error cases from function execution
func (e *FunctionExecutor) processResult(
	functionKey string,
	cb CircuitBreaker,
	entrypoint string,
	result callResult,
	startTime time.Time,
) ([]byte, error) {
	execTime := time.Since(startTime)
	
	// Handle error case
	if result.err != nil {
		// Record failure in circuit breaker
		isOpen := cb.RecordFailure()
		
		// Log if circuit breaker opened
		if isOpen {
			e.logCircuitBreakerOpen(functionKey)
		}
		
		return nil, e.logAndWrapError(functionKey, "failed to call function", result.err)
	}
	
	// Handle success case
	e.logStore.AddLog(functionKey, logging.LevelInfo,
		fmt.Sprintf("Function executed successfully: %s (execution time: %v, response size: %d bytes)",
			entrypoint, execTime, len(result.output)))
	
	cb.RecordSuccess()
	return result.output, nil
}

// handleCancellation handles context cancellation and timeout cases
func (e *FunctionExecutor) handleCancellation(
	ctx context.Context,
	functionKey string,
	cb CircuitBreaker,
) ([]byte, error) {
	// Record the failure in the circuit breaker
	isOpen := cb.RecordFailure()
	
	// Determine the specific error message based on cancellation reason
	var operation string
	if ctx.Err() == context.DeadlineExceeded {
		operation = fmt.Sprintf("function execution timed out after %v", e.defaultTimeout)
	} else {
		operation = "function execution was cancelled"
	}
	
	// Log if circuit breaker opened
	if isOpen {
		e.logCircuitBreakerOpen(functionKey)
	}
	
	return nil, e.logAndWrapError(functionKey, operation, ctx.Err())
}

// Returns:
//   - time.Duration: The configured default timeout for function execution
func (e *FunctionExecutor) DefaultTimeout() time.Duration {
	return e.defaultTimeout
}
