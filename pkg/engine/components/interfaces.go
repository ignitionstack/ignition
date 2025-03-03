package components

import (
	"context"
	"fmt"
	"strings"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

// FunctionID represents a unique function identifier
type FunctionID struct {
	Namespace string
	Name      string
}

// String returns the string representation of a function ID
func (id FunctionID) String() string {
	return fmt.Sprintf("%s/%s", id.Namespace, id.Name)
}

// FunctionIDFromKey creates a FunctionID from a string key
func FunctionIDFromKey(key string) FunctionID {
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return FunctionID{}
	}
	return FunctionID{
		Namespace: parts[0],
		Name:      parts[1],
	}
}

// PluginOperations defines the core plugin lifecycle operations
type PluginOperations interface {
	// Get a plugin by key
	GetPlugin(key string) (*extism.Plugin, bool)

	// Store a plugin
	StorePlugin(key string, plugin *extism.Plugin, digest string, config map[string]string)

	// Remove a plugin
	RemovePlugin(key string) bool
}

// PluginStateManager defines operations for managing plugin state
type PluginStateManager interface {
	// Check if a plugin is loaded
	IsPluginLoaded(key string) bool

	// Check if a plugin was previously loaded
	WasPreviouslyLoaded(key string) (bool, map[string]string)

	// Check if config has changed
	HasConfigChanged(key string, newConfig map[string]string) bool

	// Check if digest has changed
	HasDigestChanged(key string, newDigest string) bool

	// Get plugin digest
	GetPluginDigest(key string) (string, bool)

	// Get plugin config
	GetPluginConfig(key string) (map[string]string, bool)
}

// FunctionStateController defines operations for controlling function state
type FunctionStateController interface {
	// Stop a function (prevents auto-reload)
	StopFunction(key string) bool

	// Check if a function is stopped
	IsFunctionStopped(key string) bool

	// Clear stopped status
	ClearStoppedStatus(key string)
}

// PluginLifecycleManager defines operations for plugin lifecycle management
type PluginLifecycleManager interface {
	// Start cleanup routine
	StartCleanup(ctx context.Context)

	// Shutdown and cleanup resources
	Shutdown()
}

// PluginInfoProvider defines operations for retrieving plugin information
type PluginInfoProvider interface {
	// List loaded functions
	ListLoadedFunctions() []string

	// Get loaded function count
	GetLoadedFunctionCount() int

	// Get previously loaded functions
	GetPreviouslyLoadedFunctions() map[string]bool

	// Get stopped functions
	GetStoppedFunctions() map[string]bool

	// Get log store
	GetLogStore() *logging.FunctionLogStore
}

// PluginManager combines all plugin management interfaces
// This is provided for backward compatibility and convenient usage
type PluginManager interface {
	PluginOperations
	PluginStateManager
	FunctionStateController
	PluginLifecycleManager
	PluginInfoProvider
}
