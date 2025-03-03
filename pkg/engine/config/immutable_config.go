package config

import (
	"fmt"
	"sort"
	"strings"
)

// ImmutableConfig provides an immutable key-value store for configuration.
// This avoids unnecessary copying when passing configurations around.
type ImmutableConfig struct {
	values map[string]string
}

// NewConfig creates a new ImmutableConfig from a map.
// The input map is copied to avoid external modifications.
func NewConfig(values map[string]string) ImmutableConfig {
	if values == nil {
		return ImmutableConfig{
			values: make(map[string]string),
		}
	}

	// Copy the map to ensure immutability
	copied := make(map[string]string, len(values))
	for k, v := range values {
		copied[k] = v
	}

	return ImmutableConfig{
		values: copied,
	}
}

// EmptyConfig returns an empty configuration.
func EmptyConfig() ImmutableConfig {
	return ImmutableConfig{
		values: make(map[string]string),
	}
}

// Get retrieves a value from the configuration.
func (c ImmutableConfig) Get(key string) (string, bool) {
	value, exists := c.values[key]
	return value, exists
}

// GetWithDefault retrieves a value or returns a default if not found.
func (c ImmutableConfig) GetWithDefault(key string, defaultValue string) string {
	if value, exists := c.values[key]; exists {
		return value
	}
	return defaultValue
}

// Has checks if a key exists in the configuration.
func (c ImmutableConfig) Has(key string) bool {
	_, exists := c.values[key]
	return exists
}

// Size returns the number of configuration entries.
func (c ImmutableConfig) Size() int {
	return len(c.values)
}

// IsEmpty checks if the configuration is empty.
func (c ImmutableConfig) IsEmpty() bool {
	return len(c.values) == 0
}

// Keys returns all keys in the configuration.
func (c ImmutableConfig) Keys() []string {
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Values returns all values in the configuration.
func (c ImmutableConfig) Values() []string {
	values := make([]string, 0, len(c.values))
	for _, v := range c.values {
		values = append(values, v)
	}
	return values
}

// ToMap converts the ImmutableConfig back to a regular map.
// The returned map is a copy to maintain immutability.
func (c ImmutableConfig) ToMap() map[string]string {
	result := make(map[string]string, len(c.values))
	for k, v := range c.values {
		result[k] = v
	}
	return result
}

// With returns a new ImmutableConfig with the additional key-value pair.
func (c ImmutableConfig) With(key, value string) ImmutableConfig {
	// Create a new map with the existing values
	newValues := make(map[string]string, len(c.values)+1)
	for k, v := range c.values {
		newValues[k] = v
	}

	// Add the new value
	newValues[key] = value

	return ImmutableConfig{
		values: newValues,
	}
}

// Without returns a new ImmutableConfig without the specified key.
func (c ImmutableConfig) Without(key string) ImmutableConfig {
	// If the key doesn't exist, return the original config
	if _, exists := c.values[key]; !exists {
		return c
	}

	// Create a new map without the key
	newValues := make(map[string]string, len(c.values)-1)
	for k, v := range c.values {
		if k != key {
			newValues[k] = v
		}
	}

	return ImmutableConfig{
		values: newValues,
	}
}

// Merge combines two configurations, with values from other taking precedence.
func (c ImmutableConfig) Merge(other ImmutableConfig) ImmutableConfig {
	// Create a new map with the existing values
	newValues := make(map[string]string, len(c.values)+len(other.values))

	// Copy values from this config
	for k, v := range c.values {
		newValues[k] = v
	}

	// Override with values from other config
	for k, v := range other.values {
		newValues[k] = v
	}

	return ImmutableConfig{
		values: newValues,
	}
}

// Filter returns a new ImmutableConfig with only keys that match the predicate.
func (c ImmutableConfig) Filter(predicate func(key, value string) bool) ImmutableConfig {
	newValues := make(map[string]string)

	for k, v := range c.values {
		if predicate(k, v) {
			newValues[k] = v
		}
	}

	return ImmutableConfig{
		values: newValues,
	}
}

// WithPrefix returns a new ImmutableConfig with only keys that have the given prefix.
// The prefix is removed from the keys in the result.
func (c ImmutableConfig) WithPrefix(prefix string) ImmutableConfig {
	newValues := make(map[string]string)

	for k, v := range c.values {
		if strings.HasPrefix(k, prefix) {
			newKey := strings.TrimPrefix(k, prefix)
			newValues[newKey] = v
		}
	}

	return ImmutableConfig{
		values: newValues,
	}
}

// Equals checks if two configurations are equal.
func (c ImmutableConfig) Equals(other ImmutableConfig) bool {
	if len(c.values) != len(other.values) {
		return false
	}

	for k, v := range c.values {
		if otherV, exists := other.values[k]; !exists || otherV != v {
			return false
		}
	}

	return true
}

// String returns a string representation of the configuration.
func (c ImmutableConfig) String() string {
	var builder strings.Builder

	keys := c.Keys() // Get sorted keys

	builder.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(fmt.Sprintf("%s: %s", k, c.values[k]))
	}
	builder.WriteString("}")

	return builder.String()
}
