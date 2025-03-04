package state

import (
	"fmt"
	"strings"

	"github.com/ignitionstack/ignition/pkg/engine/interfaces"
)

// DefaultKeyHandler implements interfaces.KeyHandler
type DefaultKeyHandler struct{}

// NewKeyHandler creates a new DefaultKeyHandler
func NewKeyHandler() *DefaultKeyHandler {
	return &DefaultKeyHandler{}
}

// GetKey returns a string key for a namespace and name
func (h *DefaultKeyHandler) GetKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// ParseKey parses a string key into namespace and name
func (h *DefaultKeyHandler) ParseKey(key string) (namespace string, name string, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// CreateFunctionKey creates a new FunctionKey from namespace and name
func (h *DefaultKeyHandler) CreateFunctionKey(namespace, name string) interfaces.FunctionKey {
	return interfaces.FunctionKey{
		Namespace: namespace,
		Name:      name,
	}
}

// FunctionKeyFromString creates a FunctionKey from a string key
func (h *DefaultKeyHandler) FunctionKeyFromString(key string) (interfaces.FunctionKey, error) {
	namespace, name, ok := h.ParseKey(key)
	if !ok {
		return interfaces.FunctionKey{}, fmt.Errorf("invalid function key format: %s", key)
	}
	return interfaces.FunctionKey{
		Namespace: namespace,
		Name:      name,
	}, nil
}
