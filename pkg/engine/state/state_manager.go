package state

import (
	"sync"

	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
)

// FunctionState represents the state of a function
type functionState struct {
	digest           string
	config           map[string]string
	loaded           bool
	stopped          bool
	previouslyLoaded bool
	// Removed unused runtime field
}

// DefaultStateManager implements interfaces.StateManager
type DefaultStateManager struct {
	mu         sync.RWMutex
	states     map[string]*functionState
	runtimes   map[string]interfaces.WasmRuntime
	keyHandler interfaces.KeyHandler
}

// NewStateManager creates a new DefaultStateManager
func NewStateManager(keyHandler interfaces.KeyHandler) *DefaultStateManager {
	return &DefaultStateManager{
		states:     make(map[string]*functionState),
		runtimes:   make(map[string]interfaces.WasmRuntime),
		keyHandler: keyHandler,
	}
}

// GetState implements interfaces.StateManager
func (m *DefaultStateManager) GetState(namespace, name string) interfaces.FunctionState {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[key]
	if !exists {
		return interfaces.FunctionState{}
	}

	// Convert internal state to public interface state
	runtime := m.runtimes[key]
	result := interfaces.FunctionState{
		Loaded:           state.loaded,
		Stopped:          state.stopped,
		PreviouslyLoaded: state.previouslyLoaded,
		Config:           copyConfigMap(state.config),
		Running:          state.loaded && !state.stopped, // Simplified; in practice would check circuit breaker
		Digest:           state.digest,
	}

	// Add runtime info if available
	if runtime != nil {
		info := runtime.GetInfo()
		result.Digest = info.Digest
	}

	return result
}

// IsLoaded implements interfaces.StateManager
func (m *DefaultStateManager) IsLoaded(namespace, name string) bool {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[key]
	return exists && state.loaded
}

// IsStopped implements interfaces.StateManager
func (m *DefaultStateManager) IsStopped(namespace, name string) bool {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[key]
	return exists && state.stopped
}

// WasPreviouslyLoaded implements interfaces.StateManager
func (m *DefaultStateManager) WasPreviouslyLoaded(namespace, name string) (bool, map[string]string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[key]
	if !exists {
		return false, nil
	}

	return state.previouslyLoaded, copyConfigMap(state.config)
}

// MarkLoaded implements interfaces.StateManager
func (m *DefaultStateManager) MarkLoaded(namespace, name, digest string, config map[string]string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[key]
	if !exists {
		state = &functionState{}
		m.states[key] = state
	}

	state.loaded = true
	state.previouslyLoaded = true
	state.digest = digest
	state.config = copyConfigMap(config)
}

// MarkUnloaded implements interfaces.StateManager
func (m *DefaultStateManager) MarkUnloaded(namespace, name string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[key]
	if !exists {
		return
	}

	state.loaded = false

	// Remove runtime
	delete(m.runtimes, key)
}

// MarkStopped implements interfaces.StateManager
func (m *DefaultStateManager) MarkStopped(namespace, name string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[key]
	if !exists {
		state = &functionState{}
		m.states[key] = state
	}

	state.stopped = true
	state.loaded = false

	// Remove runtime
	delete(m.runtimes, key)
}

// ClearStoppedStatus implements interfaces.StateManager
func (m *DefaultStateManager) ClearStoppedStatus(namespace, name string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[key]
	if !exists {
		return
	}

	state.stopped = false
}

// GetDigest implements interfaces.StateManager
func (m *DefaultStateManager) GetDigest(namespace, name string) (string, bool) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[key]
	if !exists {
		return "", false
	}

	return state.digest, true
}

// ListLoaded implements interfaces.StateManager
func (m *DefaultStateManager) ListLoaded() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, 0, len(m.states))

	for key, state := range m.states {
		if state.loaded {
			result = append(result, key)
		}
	}

	return result
}

// Helper function to make a deep copy of a config map
func copyConfigMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
}

// StoreRuntime stores a runtime for a function
func (m *DefaultStateManager) StoreRuntime(namespace, name string, runtime interfaces.WasmRuntime) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.runtimes[key] = runtime

	// Update state info
	state, exists := m.states[key]
	if !exists {
		state = &functionState{}
		m.states[key] = state
	}

	info := runtime.GetInfo()
	state.digest = info.Digest
	state.loaded = true
	state.previouslyLoaded = true
}

// GetRuntime retrieves a runtime for a function
func (m *DefaultStateManager) GetRuntime(namespace, name string) (interfaces.WasmRuntime, bool) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	runtime, exists := m.runtimes[key]
	return runtime, exists
}

// RemoveRuntime removes a runtime for a function
func (m *DefaultStateManager) RemoveRuntime(namespace, name string) {
	key := m.keyHandler.GetKey(namespace, name)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.runtimes, key)

	// Update state
	state, exists := m.states[key]
	if exists {
		state.loaded = false
	}
}
