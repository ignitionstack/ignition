package engine

import (
	"time"

	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/engine/config"
	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	engineServices "github.com/ignitionstack/ignition/pkg/engine/services"
	"github.com/ignitionstack/ignition/pkg/engine/state"
	"github.com/ignitionstack/ignition/pkg/engine/wasm"
	"github.com/ignitionstack/ignition/pkg/registry"
	"go.uber.org/dig"
)

// BuildContainer builds the dependency injection container with all services.
func BuildContainer(cfg *config.Config, logger logging.Logger) (*dig.Container, error) {
	container := dig.New()

	// Register configuration
	if err := container.Provide(func() *config.Config {
		return cfg
	}); err != nil {
		return nil, err
	}

	// Register logger
	if err := container.Provide(func() logging.Logger {
		return logger
	}); err != nil {
		return nil, err
	}

	// Register log store
	if err := container.Provide(func() *logging.FunctionLogStore {
		return logging.NewFunctionLogStore(1000) // Store up to 1000 logs
	}); err != nil {
		return nil, err
	}

	// Register key handler
	if err := container.Provide(func() interfaces.KeyHandler {
		return state.NewKeyHandler()
	}); err != nil {
		return nil, err
	}

	// Register state manager
	if err := container.Provide(func(keyHandler interfaces.KeyHandler) interfaces.StateManager {
		return state.NewStateManager(keyHandler)
	}); err != nil {
		return nil, err
	}

	// Register circuit breaker
	if err := container.Provide(func(keyHandler interfaces.KeyHandler) interfaces.CircuitBreakerManager {
		return state.NewCircuitBreakerManager(keyHandler, 5, 30*time.Second)
	}); err != nil {
		return nil, err
	}

	// Register metrics collector
	if err := container.Provide(func(logger logging.Logger) interfaces.MetricsCollector {
		return engineServices.NewMetricsCollector(logger)
	}); err != nil {
		return nil, err
	}

	// Register runtime factory
	if err := container.Provide(func() interfaces.WasmRuntimeFactory {
		return &wasm.ExtismRuntimeFactory{}
	}); err != nil {
		return nil, err
	}

	// Register execution service
	if err := container.Provide(func(
		stateManager interfaces.StateManager,
		circuitBreaker interfaces.CircuitBreakerManager,
		logger logging.Logger,
		logStore *logging.FunctionLogStore,
		keyHandler interfaces.KeyHandler,
		metricsCollector interfaces.MetricsCollector,
		cfg *config.Config,
	) interfaces.ExecutionService {
		return engineServices.NewExecutionService(
			stateManager,
			circuitBreaker,
			logger,
			logStore,
			keyHandler,
			metricsCollector,
			cfg.Engine.DefaultTimeout,
		)
	}); err != nil {
		return nil, err
	}

	// Register function service
	if err := container.Provide(func(
		stateManager interfaces.StateManager,
		executionService interfaces.ExecutionService,
		registry registry.Registry,
		functionSvc services.FunctionService,
		logger logging.Logger,
		logStore *logging.FunctionLogStore,
		keyHandler interfaces.KeyHandler,
		runtimeFactory interfaces.WasmRuntimeFactory,
		circuitBreaker interfaces.CircuitBreakerManager,
		cfg *config.Config,
	) interfaces.FunctionService {
		return engineServices.NewFunctionService(
			stateManager,
			executionService,
			registry,
			functionSvc,
			logger,
			logStore,
			keyHandler,
			runtimeFactory,
			circuitBreaker,
			cfg.Engine.DefaultTimeout,
		)
	}, dig.As(new(interfaces.FunctionService))); err != nil {
		return nil, err
	}

	return container, nil
}

// GetFunctionService retrieves the FunctionService from the container.
func GetFunctionService(container *dig.Container) (interfaces.FunctionService, error) {
	var service interfaces.FunctionService
	if err := container.Invoke(func(svc interfaces.FunctionService) {
		service = svc
	}); err != nil {
		return nil, err
	}
	return service, nil
}

// GetStateManager retrieves the StateManager from the container.
func GetStateManager(container *dig.Container) (interfaces.StateManager, error) {
	var manager interfaces.StateManager
	if err := container.Invoke(func(mgr interfaces.StateManager) {
		manager = mgr
	}); err != nil {
		return nil, err
	}
	return manager, nil
}

// GetExecutionService retrieves the ExecutionService from the container.
func GetExecutionService(container *dig.Container) (interfaces.ExecutionService, error) {
	var service interfaces.ExecutionService
	if err := container.Invoke(func(svc interfaces.ExecutionService) {
		service = svc
	}); err != nil {
		return nil, err
	}
	return service, nil
}

// GetMetricsCollector retrieves the MetricsCollector from the container.
func GetMetricsCollector(container *dig.Container) (interfaces.MetricsCollector, error) {
	var collector interfaces.MetricsCollector
	if err := container.Invoke(func(col interfaces.MetricsCollector) {
		collector = col
	}); err != nil {
		return nil, err
	}
	return collector, nil
}
