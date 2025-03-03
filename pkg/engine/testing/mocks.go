package testing

import (
	"context"
	"fmt"
	"sync"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
)

// Constants for common circuit breaker states
const (
	circuitStateClosed = "closed"
	circuitStateOpen   = "open"
	
	// Default capacity for log store
	defaultLogStoreCapacity = 100
)

// MockPluginManager is a mock implementation of PluginManager for testing.
type MockPluginManager struct {
	plugins map[string]*extism.Plugin
	mutex   sync.RWMutex

	// Function call tracking for assertions
	Calls struct {
		GetPlugin                    []string
		StorePlugin                  []string
		RemovePlugin                 []string
		StopFunction                 []string
		IsFunctionStopped            []string
		ClearStoppedStatus           []string
		IsPluginLoaded               []string
		WasPreviouslyLoaded          []string
		HasConfigChanged             []string
		HasDigestChanged             []string
		GetPluginDigest              []string
		GetPluginConfig              []string
		StartCleanup                 int
		Shutdown                     int
		GetLogStore                  int
		ListLoadedFunctions          int
		GetLoadedFunctionCount       int
		GetPreviouslyLoadedFunctions int
		GetStoppedFunctions          int
	}

	// Mock behavior configuration
	Behavior struct {
		IsStoppedFunc           func(key string) bool
		WasPreviouslyLoadedFunc func(key string) (bool, map[string]string)
		HasConfigChangedFunc    func(key string, newConfig map[string]string) bool
		HasDigestChangedFunc    func(key string, newDigest string) bool
	}

	// Storage for function state
	FunctionState struct {
		stopped          map[string]bool
		previouslyLoaded map[string]bool
		configs          map[string]map[string]string
		digests          map[string]string
	}

	logStore *logging.FunctionLogStore
}

// NewMockPluginManager creates a new mock plugin manager.
func NewMockPluginManager() *MockPluginManager {
	return &MockPluginManager{
		plugins: make(map[string]*extism.Plugin),
		FunctionState: struct {
			stopped          map[string]bool
			previouslyLoaded map[string]bool
			configs          map[string]map[string]string
			digests          map[string]string
		}{
			stopped:          make(map[string]bool),
			previouslyLoaded: make(map[string]bool),
			configs:          make(map[string]map[string]string),
			digests:          make(map[string]string),
		},
		logStore: logging.NewFunctionLogStore(defaultLogStoreCapacity),
	}
}

// GetPlugin implements PluginManager.GetPlugin.
func (m *MockPluginManager) GetPlugin(key string) (*extism.Plugin, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetPlugin = append(m.Calls.GetPlugin, key)

	plugin, exists := m.plugins[key]
	return plugin, exists
}

// StorePlugin implements PluginManager.StorePlugin.
func (m *MockPluginManager) StorePlugin(key string, plugin *extism.Plugin, digest string, config map[string]string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.StorePlugin = append(m.Calls.StorePlugin, key)

	m.plugins[key] = plugin
	m.FunctionState.previouslyLoaded[key] = true

	if digest != "" {
		m.FunctionState.digests[key] = digest
	}

	if config != nil {
		// Copy the config to avoid mutation
		configCopy := make(map[string]string, len(config))
		for k, v := range config {
			configCopy[k] = v
		}
		m.FunctionState.configs[key] = configCopy
	}
}

// RemovePlugin implements PluginManager.RemovePlugin
func (m *MockPluginManager) RemovePlugin(key string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.RemovePlugin = append(m.Calls.RemovePlugin, key)

	_, exists := m.plugins[key]
	if exists {
		delete(m.plugins, key)
		return true
	}

	return false
}

// StopFunction implements PluginManager.StopFunction
func (m *MockPluginManager) StopFunction(key string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.StopFunction = append(m.Calls.StopFunction, key)

	removed := false
	if _, exists := m.plugins[key]; exists {
		delete(m.plugins, key)
		removed = true
	}

	m.FunctionState.stopped[key] = true

	return removed
}

// IsFunctionStopped implements PluginManager.IsFunctionStopped
func (m *MockPluginManager) IsFunctionStopped(key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.IsFunctionStopped = append(m.Calls.IsFunctionStopped, key)

	if m.Behavior.IsStoppedFunc != nil {
		return m.Behavior.IsStoppedFunc(key)
	}

	return m.FunctionState.stopped[key]
}

// ClearStoppedStatus implements PluginManager.ClearStoppedStatus
func (m *MockPluginManager) ClearStoppedStatus(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.ClearStoppedStatus = append(m.Calls.ClearStoppedStatus, key)

	delete(m.FunctionState.stopped, key)
}

// IsPluginLoaded implements PluginManager.IsPluginLoaded
func (m *MockPluginManager) IsPluginLoaded(key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.IsPluginLoaded = append(m.Calls.IsPluginLoaded, key)

	_, exists := m.plugins[key]
	return exists
}

// WasPreviouslyLoaded implements PluginManager.WasPreviouslyLoaded
func (m *MockPluginManager) WasPreviouslyLoaded(key string) (bool, map[string]string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.WasPreviouslyLoaded = append(m.Calls.WasPreviouslyLoaded, key)

	if m.Behavior.WasPreviouslyLoadedFunc != nil {
		return m.Behavior.WasPreviouslyLoadedFunc(key)
	}

	wasLoaded := m.FunctionState.previouslyLoaded[key]

	var config map[string]string
	if storedConfig, exists := m.FunctionState.configs[key]; exists {
		// Copy the config to avoid mutation
		config = make(map[string]string, len(storedConfig))
		for k, v := range storedConfig {
			config[k] = v
		}
	}

	return wasLoaded, config
}

// HasConfigChanged implements PluginManager.HasConfigChanged
func (m *MockPluginManager) HasConfigChanged(key string, newConfig map[string]string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.HasConfigChanged = append(m.Calls.HasConfigChanged, key)

	if m.Behavior.HasConfigChangedFunc != nil {
		return m.Behavior.HasConfigChangedFunc(key, newConfig)
	}

	currentConfig, exists := m.FunctionState.configs[key]
	if !exists {
		return true
	}

	if len(currentConfig) != len(newConfig) {
		return true
	}

	for k, v := range newConfig {
		if currentValue, exists := currentConfig[k]; !exists || currentValue != v {
			return true
		}
	}

	return false
}

// HasDigestChanged implements PluginManager.HasDigestChanged
func (m *MockPluginManager) HasDigestChanged(key string, newDigest string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.HasDigestChanged = append(m.Calls.HasDigestChanged, key)

	if m.Behavior.HasDigestChangedFunc != nil {
		return m.Behavior.HasDigestChangedFunc(key, newDigest)
	}

	currentDigest, exists := m.FunctionState.digests[key]
	return !exists || currentDigest != newDigest
}

// GetPluginDigest implements PluginManager.GetPluginDigest
func (m *MockPluginManager) GetPluginDigest(key string) (string, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetPluginDigest = append(m.Calls.GetPluginDigest, key)

	digest, exists := m.FunctionState.digests[key]
	return digest, exists
}

// GetPluginConfig implements PluginManager.GetPluginConfig
func (m *MockPluginManager) GetPluginConfig(key string) (map[string]string, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetPluginConfig = append(m.Calls.GetPluginConfig, key)

	config, exists := m.FunctionState.configs[key]

	var configCopy map[string]string
	if exists {
		// Copy the config to avoid mutation
		configCopy = make(map[string]string, len(config))
		for k, v := range config {
			configCopy[k] = v
		}
	}

	return configCopy, exists
}

// StartCleanup implements PluginManager.StartCleanup
func (m *MockPluginManager) StartCleanup(_ context.Context) {
	m.Calls.StartCleanup++
	// Do nothing in the mock
}

// Shutdown implements PluginManager.Shutdown
func (m *MockPluginManager) Shutdown() {
	m.Calls.Shutdown++
	// Do nothing in the mock
}

// GetLogStore implements PluginManager.GetLogStore
func (m *MockPluginManager) GetLogStore() *logging.FunctionLogStore {
	m.Calls.GetLogStore++
	return m.logStore
}

// ListLoadedFunctions implements PluginManager.ListLoadedFunctions
func (m *MockPluginManager) ListLoadedFunctions() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.ListLoadedFunctions++

	keys := make([]string, 0, len(m.plugins))
	for key := range m.plugins {
		keys = append(keys, key)
	}

	return keys
}

// GetLoadedFunctionCount implements PluginManager.GetLoadedFunctionCount
func (m *MockPluginManager) GetLoadedFunctionCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetLoadedFunctionCount++

	return len(m.plugins)
}

// GetPreviouslyLoadedFunctions implements PluginManager.GetPreviouslyLoadedFunctions
func (m *MockPluginManager) GetPreviouslyLoadedFunctions() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetPreviouslyLoadedFunctions++

	// Copy the map to avoid mutation
	result := make(map[string]bool, len(m.FunctionState.previouslyLoaded))
	for k, v := range m.FunctionState.previouslyLoaded {
		result[k] = v
	}

	return result
}

// GetStoppedFunctions implements PluginManager.GetStoppedFunctions
func (m *MockPluginManager) GetStoppedFunctions() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetStoppedFunctions++

	// Copy the map to avoid mutation
	result := make(map[string]bool, len(m.FunctionState.stopped))
	for k, v := range m.FunctionState.stopped {
		result[k] = v
	}

	return result
}

// MockCircuitBreaker is a mock implementation of CircuitBreaker for testing
type MockCircuitBreaker struct {
	State        string
	FailureCount int

	// Function call tracking for assertions
	Calls struct {
		RecordSuccess   int
		RecordFailure   int
		IsOpen          int
		Reset           int
		GetState        int
		GetFailureCount int
	}

	// Mock behavior configuration
	Behavior struct {
		IsOpenFunc        func() bool
		RecordFailureFunc func() bool
	}
}

// NewMockCircuitBreaker creates a new mock circuit breaker
func NewMockCircuitBreaker() *MockCircuitBreaker {
	return &MockCircuitBreaker{
		State: circuitStateClosed,
	}
}

// RecordSuccess implements CircuitBreaker.RecordSuccess
func (m *MockCircuitBreaker) RecordSuccess() {
	m.Calls.RecordSuccess++

	if m.State == "half-open" {
		m.State = circuitStateClosed
		m.FailureCount = 0
	}
}

// RecordFailure implements CircuitBreaker.RecordFailure
func (m *MockCircuitBreaker) RecordFailure() bool {
	m.Calls.RecordFailure++

	if m.Behavior.RecordFailureFunc != nil {
		return m.Behavior.RecordFailureFunc()
	}

	m.FailureCount++

	if m.State == circuitStateClosed && m.FailureCount >= 5 {
		m.State = circuitStateOpen
	}

	return m.State == circuitStateOpen
}

// IsOpen implements CircuitBreaker.IsOpen
func (m *MockCircuitBreaker) IsOpen() bool {
	m.Calls.IsOpen++

	if m.Behavior.IsOpenFunc != nil {
		return m.Behavior.IsOpenFunc()
	}

	return m.State == circuitStateOpen
}

// Reset implements CircuitBreaker.Reset
func (m *MockCircuitBreaker) Reset() {
	m.Calls.Reset++

	m.State = circuitStateClosed
	m.FailureCount = 0
}

// GetState implements CircuitBreaker.GetState
func (m *MockCircuitBreaker) GetState() string {
	m.Calls.GetState++

	return m.State
}

// GetFailureCount implements CircuitBreaker.GetFailureCount
func (m *MockCircuitBreaker) GetFailureCount() int {
	m.Calls.GetFailureCount++

	return m.FailureCount
}

// MockCircuitBreakerManager is a mock implementation of CircuitBreakerManager for testing
type MockCircuitBreakerManager struct {
	circuitBreakers map[string]*MockCircuitBreaker
	mutex           sync.RWMutex

	// Function call tracking for assertions
	Calls struct {
		GetCircuitBreaker      []string
		RemoveCircuitBreaker   []string
		Reset                  int
		GetCircuitBreakerState []string
		GetAllCircuitBreakers  int
	}
}

// NewMockCircuitBreakerManager creates a new mock circuit breaker manager
func NewMockCircuitBreakerManager() *MockCircuitBreakerManager {
	return &MockCircuitBreakerManager{
		circuitBreakers: make(map[string]*MockCircuitBreaker),
	}
}

// GetCircuitBreaker implements CircuitBreakerManager.GetCircuitBreaker
func (m *MockCircuitBreakerManager) GetCircuitBreaker(key string) components.CircuitBreaker {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.GetCircuitBreaker = append(m.Calls.GetCircuitBreaker, key)

	cb, exists := m.circuitBreakers[key]
	if !exists {
		cb = NewMockCircuitBreaker()
		m.circuitBreakers[key] = cb
	}

	return cb
}

// RemoveCircuitBreaker implements CircuitBreakerManager.RemoveCircuitBreaker
func (m *MockCircuitBreakerManager) RemoveCircuitBreaker(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.RemoveCircuitBreaker = append(m.Calls.RemoveCircuitBreaker, key)

	delete(m.circuitBreakers, key)
}

// Reset implements CircuitBreakerManager.Reset
func (m *MockCircuitBreakerManager) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Calls.Reset++

	for _, cb := range m.circuitBreakers {
		cb.Reset()
	}
}

// GetCircuitBreakerState implements CircuitBreakerManager.GetCircuitBreakerState
func (m *MockCircuitBreakerManager) GetCircuitBreakerState(key string) string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetCircuitBreakerState = append(m.Calls.GetCircuitBreakerState, key)

	cb, exists := m.circuitBreakers[key]
	if !exists {
		return ""
	}

	return cb.GetState()
}

// GetAllCircuitBreakers implements CircuitBreakerManager.GetAllCircuitBreakers
func (m *MockCircuitBreakerManager) GetAllCircuitBreakers() map[string]components.CircuitBreaker {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.Calls.GetAllCircuitBreakers++

	result := make(map[string]components.CircuitBreaker, len(m.circuitBreakers))
	for key, cb := range m.circuitBreakers {
		result[key] = cb
	}

	return result
}

// MockRegistry is a mock implementation of Registry for testing
type MockRegistry struct {
	Functions map[string]map[string]*registry.FunctionMetadata
	Versions  map[string]map[string]map[string]*registry.VersionInfo
	Tags      map[string]map[string]map[string]string // namespace -> name -> tag -> digest

	// Function call tracking for assertions
	Calls struct {
		Pull              []string
		Push              []string
		List              []string
		CreateFunction    []string
		GetFunction       []string
		GetVersion        []string
		GetTag            []string
		Tag               []string
		UntagVersion      []string
		GetDefaultVersion []string
		SetDefaultVersion []string
	}

	// Mock behavior configuration
	Behavior struct {
		PullFunc              func(namespace, name, reference string) ([]byte, *registry.VersionInfo, error)
		PushFunc              func(namespace, name string, wasm []byte, versionInfo *registry.VersionInfo) (string, error)
		ListFunc              func(namespace string) ([]registry.FunctionMetadata, error)
		CreateFunctionFunc    func(namespace, name string, info *registry.FunctionMetadata) error
		GetFunctionFunc       func(namespace, name string) (*registry.FunctionMetadata, error)
		GetVersionFunc        func(namespace, name, version string) (*registry.VersionInfo, error)
		GetTagFunc            func(namespace, name, tag string) (string, error)
		TagFunc               func(namespace, name, tag, version string) error
		UntagVersionFunc      func(namespace, name, tag string) error
		GetDefaultVersionFunc func(namespace, name string) (string, error)
		SetDefaultVersionFunc func(namespace, name, version string) error
	}
}

// NewMockRegistry creates a new mock registry
func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		Functions: make(map[string]map[string]*registry.FunctionMetadata),
		Versions:  make(map[string]map[string]map[string]*registry.VersionInfo),
		Tags:      make(map[string]map[string]map[string]string),
	}
}


// Pull implements Registry.Pull
func (m *MockRegistry) Pull(namespace, name, reference string) ([]byte, *registry.VersionInfo, error) {
	key := fmt.Sprintf("%s/%s/%s", namespace, name, reference)
	m.Calls.Pull = append(m.Calls.Pull, key)

	if m.Behavior.PullFunc != nil {
		return m.Behavior.PullFunc(namespace, name, reference)
	}

	// Mock implementation returning empty WASM bytes and a simple version info
	return []byte("mock wasm bytes"), &registry.VersionInfo{
		Hash:     "mock-digest",
		Settings: manifest.FunctionVersionSettings{},
	}, nil
}

// Additional methods would be implemented similarly

// MockLogger is a mock implementation of logging.Logger for testing
type MockLogger struct {
	logs []string

	// Function call tracking for assertions
	Calls struct {
		Printf int
	}
}

// NewMockLogger creates a new mock logger
func NewMockLogger() *MockLogger {
	return &MockLogger{
		logs: make([]string, 0),
	}
}

// Printf implements logging.Logger.Printf
func (m *MockLogger) Printf(format string, v ...interface{}) {
	m.Calls.Printf++

	message := fmt.Sprintf(format, v...)
	m.logs = append(m.logs, message)
}

// GetLogs returns all logged messages
func (m *MockLogger) GetLogs() []string {
	return m.logs
}

// Clear clears all logged messages
func (m *MockLogger) Clear() {
	m.logs = make([]string, 0)
}

// TestFixture provides a complete test fixture for engine tests
type TestFixture struct {
	PluginManager  *MockPluginManager
	CircuitBreaker *MockCircuitBreakerManager
	Registry       *MockRegistry
	Logger         *MockLogger
}

// NewTestFixture creates a new test fixture
func NewTestFixture() *TestFixture {
	return &TestFixture{
		PluginManager:  NewMockPluginManager(),
		CircuitBreaker: NewMockCircuitBreakerManager(),
		Registry:       NewMockRegistry(),
		Logger:         NewMockLogger(),
	}
}
