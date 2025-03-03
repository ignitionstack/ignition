package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

// FunctionExecutor is responsible for executing functions and managing circuit breakers
type FunctionExecutor struct {
	pluginManager   *components.PluginManager
	circuitBreakers *components.CircuitBreakerManager
	logStore        *logging.FunctionLogStore
	logger          logging.Logger
	defaultTimeout  time.Duration
}

// NewFunctionExecutor creates a new function executor
func NewFunctionExecutor(pluginManager *components.PluginManager, circuitBreakers *components.CircuitBreakerManager, 
	logStore *logging.FunctionLogStore, logger logging.Logger, defaultTimeout time.Duration) *FunctionExecutor {
	return &FunctionExecutor{
		pluginManager:   pluginManager,
		circuitBreakers: circuitBreakers,
		logStore:        logStore,
		logger:          logger,
		defaultTimeout:  defaultTimeout,
	}
}

// CallFunction calls a function with context
func (e *FunctionExecutor) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error) {
	functionKey := components.GetFunctionKey(namespace, name)

	e.logStore.AddLog(functionKey, logging.LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes", entrypoint, len(payload)))

	// Circuit breaker check
	cb := e.circuitBreakers.GetCircuitBreaker(functionKey)
	if cb.IsOpen() {
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", functionKey)
		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Get the plugin
	plugin, ok := e.pluginManager.GetPlugin(functionKey)
	if !ok {
		e.logStore.AddLog(functionKey, logging.LevelError, "Function not loaded")
		return nil, ErrFunctionNotLoaded
	}

	startTime := time.Now()

	// Result channel with buffer to prevent goroutine leaks
	resultCh := make(chan struct {
		output []byte
		err    error
	}, 1)

	// Cancel context for the goroutine if this function returns
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Execute the plugin call in a goroutine
	go func() {
		_, output, err := plugin.Call(entrypoint, payload)

		// Send the result, handling the case where the context is cancelled
		select {
		case resultCh <- struct {
			output []byte
			err    error
		}{output, err}:
			// Result sent successfully
		case <-execCtx.Done():
			// Context was cancelled, nothing to do
		}
	}()

	// Wait for the result or context cancellation
	select {
	case result := <-resultCh:
		if result.err != nil {
			// Record the failure in the circuit breaker
			isOpen := cb.RecordFailure()
			errMsg := fmt.Sprintf("Failed to call function: %v", result.err)
			e.logStore.AddLog(functionKey, logging.LevelError, errMsg)

			if isOpen {
				cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
				e.logger.Printf(cbMsg)
				e.logStore.AddLog(functionKey, logging.LevelError, cbMsg)
			}

			return nil, fmt.Errorf("failed to call function: %w", result.err)
		}

		// Record success in metrics and logs
		execTime := time.Since(startTime)
		e.logStore.AddLog(functionKey, logging.LevelInfo,
			fmt.Sprintf("Function executed successfully: %s (execution time: %v, response size: %d bytes)",
				entrypoint, execTime, len(result.output)))

		cb.RecordSuccess()
		return result.output, nil

	case <-ctx.Done():
		// The context deadline was exceeded or cancelled
		isOpen := cb.RecordFailure()

		// Determine the specific error
		var errMsg string
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("Function execution timed out after %v", e.defaultTimeout)
		} else {
			errMsg = "Function execution was cancelled"
		}

		e.logStore.AddLog(functionKey, logging.LevelError, errMsg)

		if isOpen {
			cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", functionKey)
			e.logger.Printf(cbMsg)
			e.logStore.AddLog(functionKey, logging.LevelError, cbMsg)
		}

		return nil, fmt.Errorf("%s", errMsg)
	}
}

// DefaultTimeout returns the default timeout for function calls
func (e *FunctionExecutor) DefaultTimeout() time.Duration {
	return e.defaultTimeout
}