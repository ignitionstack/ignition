package components

import (
	"context"
	"fmt"
	"strings"

	extism "github.com/extism/go-sdk"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
)

// FunctionID represents a unique function identifier.
type FunctionID struct {
	Namespace string
	Name      string
}

// String returns the string representation of a function ID.
func (id FunctionID) String() string {
	return fmt.Sprintf("%s/%s", id.Namespace, id.Name)
}

// FunctionIDFromKey creates a FunctionID from a string key.
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

// PluginManager defines all plugin management capabilities
type PluginManager interface {
	// Plugin operations
	GetPlugin(key string) (*extism.Plugin, bool)
	StorePlugin(key string, plugin *extism.Plugin, digest string, config map[string]string)
	RemovePlugin(key string) bool

	// Plugin state management
	IsPluginLoaded(key string) bool
	WasPreviouslyLoaded(key string) (bool, map[string]string)
	HasConfigChanged(key string, newConfig map[string]string) bool
	HasDigestChanged(key string, newDigest string) bool
	GetPluginDigest(key string) (string, bool)
	GetPluginConfig(key string) (map[string]string, bool)

	// Function state control
	StopFunction(key string) bool
	IsFunctionStopped(key string) bool
	ClearStoppedStatus(key string)

	// Lifecycle management
	StartCleanup(ctx context.Context)
	Shutdown()

	// Information provider
	ListLoadedFunctions() []string
	GetLoadedFunctionCount() int
	GetPreviouslyLoadedFunctions() map[string]bool
	GetStoppedFunctions() map[string]bool
	GetLogStore() *logging.FunctionLogStore
}
