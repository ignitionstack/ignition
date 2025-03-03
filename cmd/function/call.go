package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

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

			req := map[string]interface{}{
				"namespace":  namespace,
				"name":       name,
				"reference":  reference,
				"entrypoint": entrypoint,
				"payload":    payload,
			}

			// Only add config if there are values
			if len(config) > 0 {
				req["config"] = config
			}

			client := http.Client{
				Transport: &http.Transport{
					Dial: func(_, _ string) (net.Conn, error) {
						return net.Dial("unix", callSocketPath)
					},
				},
			}

			reqBytes, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://unix/call-once", bytes.NewBuffer(reqBytes))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(httpReq)
			if err != nil {
				return fmt.Errorf("failed to call function: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("engine returned error (status %s): %s", resp.Status, string(bodyBytes))
			}

			// Read and print the response
			output, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			// If the output looks like JSON, pretty print it
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
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
	cmd.Flags().StringVarP(&callSocketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringArrayVarP(&callConfigFlag, "config", "c", []string{}, "Configuration values to pass to the function (format: key=value)")

	return cmd
}
