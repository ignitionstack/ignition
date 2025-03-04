package interfaces

import (
	"fmt"
)

// FunctionKey represents a unique identifier for a function
type FunctionKey struct {
	Namespace string
	Name      string
}

// String returns the string representation of a function key (namespace/name)
func (k FunctionKey) String() string {
	return fmt.Sprintf("%s/%s", k.Namespace, k.Name)
}

// NewFunctionKey creates a new FunctionKey from namespace and name
func NewFunctionKey(namespace, name string) FunctionKey {
	return FunctionKey{
		Namespace: namespace,
		Name:      name,
	}
}

// ParseFunctionKey parses a string key into a FunctionKey
func ParseFunctionKey(key string) (FunctionKey, error) {
	namespace, name, found := SplitFunctionKey(key)
	if !found {
		return FunctionKey{}, fmt.Errorf("invalid function key format: %s", key)
	}
	return FunctionKey{
		Namespace: namespace,
		Name:      name,
	}, nil
}

// SplitFunctionKey splits a string key into namespace and name components
func SplitFunctionKey(key string) (namespace, name string, ok bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}

// MetricsCollector defines the interface for collecting metrics
type MetricsCollector interface {
	// RecordExecution records a function execution
	RecordExecution(key FunctionKey, duration float64, success bool)

	// RecordMemoryUsage records memory usage for a function
	RecordMemoryUsage(key FunctionKey, bytesUsed int64)

	// RecordConcurrency records the number of concurrent function executions
	RecordConcurrency(count int)
}

// KeyHandler provides methods for working with function keys
type KeyHandler interface {
	// GetKey returns a string key for a namespace and name
	GetKey(namespace, name string) string

	// ParseKey parses a string key into namespace and name
	ParseKey(key string) (namespace string, name string, ok bool)
}
