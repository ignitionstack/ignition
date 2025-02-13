package function

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/spf13/cobra"
)

func NewFunctionTagCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "tag [namespace/name:digest] [tag]",
		Short: "Assign a tag to a specific digest",
		Args:  cobra.ExactArgs(2), // Requires exactly 2 arguments
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse namespace, name, and digest from the first argument
			namespace, name, digest, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			// The second argument is the tag to assign
			tag := args[1]

			// Send a request to the engine to assign the tag
			req := map[string]string{
				"namespace": namespace,
				"name":      name,
				"tag":       tag,
				"digest":    digest,
			}

			reqBody, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			// Create an HTTP client with Unix socket transport
			client := http.Client{
				Transport: &http.Transport{
					Dial: func(_, _ string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			}

			// Send the request to the engine
			resp, err := client.Post("http://unix/reassign-tag", "application/json", bytes.NewBuffer(reqBody))
			if err != nil {
				return fmt.Errorf("failed to send request to engine: %w", err)
			}
			defer resp.Body.Close()

			// Check the response status
			if resp.StatusCode == http.StatusNotFound {
				return fmt.Errorf("function %s not found", args[0])
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("engine returned error: %s", resp.Status)
			}

			// Print success message
			fmt.Printf("Tag %s assigned to digest %s for function %s/%s\n", tag, digest, namespace, name)
			return nil
		},
	}

	// Add the socket path flag
	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")

	return cmd
}
