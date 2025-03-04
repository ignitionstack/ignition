package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/errors"
	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

// ExecutionServiceImpl implements interfaces.ExecutionService
type ExecutionServiceImpl struct {
	stateManager     interfaces.StateManager
	circuitBreaker   interfaces.CircuitBreakerManager
	logger           logging.Logger
	logStore         *logging.FunctionLogStore
	keyHandler       interfaces.KeyHandler
	metricsCollector interfaces.MetricsCollector

	// Stats tracking
	mu             sync.RWMutex
	stats          map[string]*executionStats
	defaultTimeout time.Duration
}

type executionStats struct {
	total         int64
	successful    int64
	failed        int64
	totalTime     time.Duration
	maxTime       time.Duration
	lastExecution time.Time
}

// NewExecutionService creates a new ExecutionServiceImpl
func NewExecutionService(
	stateManager interfaces.StateManager,
	circuitBreaker interfaces.CircuitBreakerManager,
	logger logging.Logger,
	logStore *logging.FunctionLogStore,
	keyHandler interfaces.KeyHandler,
	metricsCollector interfaces.MetricsCollector,
	defaultTimeout time.Duration,
) *ExecutionServiceImpl {
	return &ExecutionServiceImpl{
		stateManager:     stateManager,
		circuitBreaker:   circuitBreaker,
		logger:           logger,
		logStore:         logStore,
		keyHandler:       keyHandler,
		metricsCollector: metricsCollector,
		stats:            make(map[string]*executionStats),
		defaultTimeout:   defaultTimeout,
	}
}

// Execute implements interfaces.ExecutionService
func (s *ExecutionServiceImpl) Execute(ctx context.Context, params interfaces.ExecutionParams) (interfaces.ExecutionResult, error) {
	key := s.keyHandler.GetKey(params.Namespace, params.Name)

	// Log the function call
	s.logStore.AddLog(key, logging.LevelInfo, fmt.Sprintf("Function call: %s with payload size %d bytes",
		params.Entrypoint, len(params.Payload)))

	// Check if function is loaded
	runtime, ok := s.stateManager.GetRuntime(params.Namespace, params.Name)
	if !ok {
		s.logStore.AddLog(key, logging.LevelError, "Function not loaded")
		return interfaces.ExecutionResult{}, errors.ErrFunctionNotLoaded.
			WithNamespace(params.Namespace).
			WithName(params.Name)
	}

	// Check circuit breaker
	cb := s.circuitBreaker.GetCircuitBreaker(params.Namespace, params.Name)
	if cb.IsOpen() {
		errMsg := fmt.Sprintf("Circuit breaker is open for function %s", key)
		s.logStore.AddLog(key, logging.LevelError, errMsg)
		return interfaces.ExecutionResult{}, errors.ErrCircuitBreakerOpen.
			WithNamespace(params.Namespace).
			WithName(params.Name)
	}

	// Apply timeout if specified
	var execCtx context.Context
	var cancel context.CancelFunc

	timeout := s.defaultTimeout
	if params.Timeout > 0 {
		timeout = params.Timeout
	}

	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		execCtx = ctx
	}

	// Execute the function
	startTime := time.Now()
	output, err := runtime.ExecuteFunction(execCtx, params.Entrypoint, params.Payload)
	execTime := time.Since(startTime)

	// Record metrics
	functionKey := interfaces.NewFunctionKey(params.Namespace, params.Name)
	s.metricsCollector.RecordExecution(functionKey, execTime.Seconds(), err == nil)

	// Update statistics
	s.recordExecution(params.Namespace, params.Name, execTime, err == nil)

	// Handle error
	if err != nil {
		// Record failure in circuit breaker
		isOpen := s.circuitBreaker.RecordFailure(params.Namespace, params.Name)

		// Log if circuit breaker opened
		if isOpen {
			cbMsg := fmt.Sprintf("Circuit breaker opened for function %s", key)
			s.logger.Printf(cbMsg)
			s.logStore.AddLog(key, logging.LevelError, cbMsg)
		}

		// Check if it was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			s.logStore.AddLog(key, logging.LevelError, fmt.Sprintf("Function execution timed out after %v", timeout))
			return interfaces.ExecutionResult{
				ExecutionTime: execTime,
				Error:         errors.ErrExecutionTimeout,
			}, errors.ErrExecutionTimeout
		}

		// Other errors
		errMsg := fmt.Sprintf("Function execution failed: %v", err)
		s.logStore.AddLog(key, logging.LevelError, errMsg)

		return interfaces.ExecutionResult{
				ExecutionTime: execTime,
				Error:         err,
			}, errors.Wrap(errors.DomainExecution, errors.CodeExecutionFailed, "Function execution failed", err).
				WithNamespace(params.Namespace).
				WithName(params.Name)
	}

	// Record success in circuit breaker
	s.circuitBreaker.RecordSuccess(params.Namespace, params.Name)

	// Log success
	s.logStore.AddLog(key, logging.LevelInfo, fmt.Sprintf("Function call succeeded in %v", execTime))

	return interfaces.ExecutionResult{
		Output:        output,
		ExecutionTime: execTime,
	}, nil
}

// recordExecution updates the execution statistics
func (s *ExecutionServiceImpl) recordExecution(namespace, name string, duration time.Duration, success bool) {
	key := s.keyHandler.GetKey(namespace, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	stat, exists := s.stats[key]
	if !exists {
		stat = &executionStats{}
		s.stats[key] = stat
	}

	stat.total++
	stat.totalTime += duration
	stat.lastExecution = time.Now()

	if duration > stat.maxTime {
		stat.maxTime = duration
	}

	if success {
		stat.successful++
	} else {
		stat.failed++
	}
}

// GetStats implements interfaces.ExecutionService
func (s *ExecutionServiceImpl) GetStats(namespace, name string) interfaces.ExecutionStats {
	key := s.keyHandler.GetKey(namespace, name)

	s.mu.RLock()
	defer s.mu.RUnlock()

	stat, exists := s.stats[key]
	if !exists {
		return interfaces.ExecutionStats{}
	}

	var avgTime time.Duration
	if stat.total > 0 {
		avgTime = stat.totalTime / time.Duration(stat.total)
	}

	return interfaces.ExecutionStats{
		TotalExecutions:      stat.total,
		SuccessfulExecutions: stat.successful,
		FailedExecutions:     stat.failed,
		AverageExecutionTime: avgTime,
		MaxExecutionTime:     stat.maxTime,
		LastExecutionTime:    stat.lastExecution,
	}
}
