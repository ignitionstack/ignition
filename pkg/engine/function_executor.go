package engine

import (
	"context"
	"fmt"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
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
	functionKey := components.GetFunctionKey(namespace, name)

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
	
	// Create a done channel to ensure goroutine cleanup
	done := make(chan struct{})
	defer close(done)
	
	// Create a buffered result channel
	resultCh := make(chan callResult, 1)
	
	// Execute the plugin call in a goroutine
	go func() {
		_, output, err := plugin.Call(entrypoint, payload)
		
		// Try to send the result, but don't block if the parent function has returned
		select {
		case resultCh <- callResult{output, err}:
			// Result sent successfully
		case <-done:
			// Parent function has returned, nothing to do
		}
	}()
	
	// Wait for the result or context cancellation
	select {
	case result := <-resultCh:
		return e.processResult(functionKey, cb, entrypoint, result, startTime)
	case <-ctx.Done():
		return e.handleCancellation(ctx, functionKey, cb)
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
		
		// Log the error
		errMsg := fmt.Sprintf("Failed to call function: %v", result.err)
		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		
		// Log if circuit breaker opened
		if isOpen {
			cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
			e.logger.Printf(cbMsg)
			e.logStore.AddLog(functionKey, logging.LevelError, cbMsg)
		}
		
		return nil, WrapEngineError("failed to call function", result.err)
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
	var errMsg string
	if ctx.Err() == context.DeadlineExceeded {
		errMsg = fmt.Sprintf("Function execution timed out after %v", e.defaultTimeout)
	} else {
		errMsg = "Function execution was cancelled"
	}
	
	// Log the error
	e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
	
	// Log if circuit breaker opened
	if isOpen {
		cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
		e.logger.Printf(cbMsg)
		e.logStore.AddLog(functionKey, logging.LevelError, cbMsg)
	}
	
	return nil, WrapEngineError(errMsg, ctx.Err())
}

// Returns:
//   - time.Duration: The configured default timeout for function execution
func (e *FunctionExecutor) DefaultTimeout() time.Duration {
	return e.defaultTimeout
}
