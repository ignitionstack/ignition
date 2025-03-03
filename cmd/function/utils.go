package function

import (
	"strings"
)

// splitKeyValue splits a string in format "key=value" into a tuple ["key", "value"].
// If the string doesn't contain "=", returns a slice with the original string.
func splitKeyValue(input string) []string {
	parts := strings.SplitN(input, "=", 2)
	return parts
}
