package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ignitionstack/ignition/pkg/engine/api"
	"github.com/ignitionstack/ignition/pkg/engine/client"
	"github.com/spf13/cobra"
)

var (
	entrypoint     string
	payload        string
	callSocketPath string
	callConfigFlag []string
)

func NewFunctionCallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call [namespace/name:reference]",
		Short: "Call a function once using a temporary plugin instance",
		Long: `Call a WebAssembly function with the specified payload.

This command creates a temporary instance of the function, sends the provided payload
to the specified entrypoint, and returns the result. The function is loaded from the
registry using the namespace/name:reference format, where:

- namespace: The function's namespace (e.g., 'default')
- name: The function's name (e.g., 'hello-world')
- reference: Either a tag (like 'latest') or a digest hash

The command requires a running engine to execute the function.`,
		Example: `  # Call a function with default entrypoint
  ignition call default/hello-world:latest

  # Call with a JSON payload
  ignition call default/hello-world:latest --payload '{"name": "World"}'

  # Call with a specific entrypoint
  ignition call default/hello-world:latest --entrypoint greet --payload '{"name": "World"}'

  # Call using function digest instead of tag
  ignition call default/hello-world:d7a8fbb307d7809469ca9abcb0082e4f`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			namespace, name, reference, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			// Parse config flag values into a map
			config := make(map[string]string)
			for _, configItem := range callConfigFlag {
				parts := splitKeyValue(configItem)
				if len(parts) == 2 {
					config[parts[0]] = parts[1]
				}
			}

			// Create engine client
			engineClient, err := client.New(client.Options{
				SocketPath: callSocketPath,
			})
			if err != nil {
				return fmt.Errorf("failed to create engine client: %w", err)
			}

			// Create request
			req := api.OneOffCallRequest{
				BaseRequest: api.BaseRequest{
					Namespace: namespace,
					Name:      name,
				},
				Reference:  reference,
				Entrypoint: entrypoint,
				Payload:    payload,
				Config:     config,
			}

			// Call function
			output, err := engineClient.OneOffCall(context.Background(), req)
			if err != nil {
				return fmt.Errorf("failed to call function: %w", err)
			}

			// Check if output looks like JSON
			if isJSON(output) {
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, output, "", "  "); err == nil {
					fmt.Println(prettyJSON.String())
					return nil
				}
			}

			// Otherwise print as string
			fmt.Println(string(output))
			return nil
		},
	}

	cmd.Flags().StringVarP(&entrypoint, "entrypoint", "e", "handler", "the entrypoint wasm function")
	cmd.Flags().StringVarP(&payload, "payload", "p", "", "the payload to send to the entrypoint")

	// Use the default socket path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	defaultSocketPath := filepath.Join(homeDir, ".ignition", "engine.sock")

	cmd.Flags().StringVarP(&callSocketPath, "socket", "s", defaultSocketPath, "Path to the Unix socket")
	cmd.Flags().StringArrayVarP(&callConfigFlag, "config", "c", []string{}, "Configuration values to pass to the function (format: key=value)")

	return cmd
}

// isJSON checks if a byte slice contains valid JSON
func isJSON(data []byte) bool {
	var js interface{}
	return json.Unmarshal(data, &js) == nil
}

// parseNamespaceAndName parses a string in the format namespace/name:tag
func parseNamespaceAndName(input string) (namespace, name, tag string, err error) {
	// Split namespace and name/tag
	parts := strings.Split(input, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid format: %s (expected namespace/name:tag)", input)
	}
	
	namespace = parts[0]
	nameRef := parts[1]
	
	// Split name and reference
	parts = strings.Split(nameRef, ":")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid format: %s (expected namespace/name:tag)", input)
	}
	
	name = parts[0]
	tag = parts[1]
	
	// Validate all parts are non-empty
	if namespace == "" || name == "" || tag == "" {
		return "", "", "", fmt.Errorf("invalid format: %s (all parts must be non-empty)", input)
	}
	
	return namespace, name, tag, nil
}