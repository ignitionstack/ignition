package function

import (
	"fmt"
	"strings"
)

// splitKeyValue splits a string in format "key=value" into a tuple ["key", "value"].
// If the string doesn't contain "=", returns a slice with the original string.
func splitKeyValue(input string) []string {
	parts := strings.SplitN(input, "=", 2)
	return parts
}

// parseNamespaceAndName parses a string in the format namespace/name:tag or namespace/name (defaults to :latest)
func parseNamespaceAndName(input string) (namespace, name, tag string, err error) {
	// Split namespace and name/tag
	parts := strings.Split(input, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid format: %s (expected namespace/name or namespace/name:tag)", input)
	}

	namespace = parts[0]
	nameRef := parts[1]

	// Split name and reference
	parts = strings.Split(nameRef, ":")
	if len(parts) == 1 {
		// No tag provided, use "latest" as default
		name = parts[0]
		tag = "latest"
	} else if len(parts) == 2 {
		// Tag provided
		name = parts[0]
		tag = parts[1]
	} else {
		return "", "", "", fmt.Errorf("invalid format: %s (expected namespace/name or namespace/name:tag)", input)
	}

	// Validate all parts are non-empty
	if namespace == "" || name == "" || tag == "" {
		return "", "", "", fmt.Errorf("invalid format: %s (all parts must be non-empty)", input)
	}

	return namespace, name, tag, nil
}
