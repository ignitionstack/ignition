package function

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

var (
	entrypoint string
	payload    string
)

func NewFunctionCallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call [namespace/name:reference]",
		Short: "Call a function once using a temporary plugin instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, name, reference, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			req := map[string]string{
				"namespace":  namespace,
				"name":       name,
				"reference":  reference,
				"entrypoint": entrypoint,
				"payload":    payload,
			}

			client := http.Client{
				Transport: &http.Transport{
					Dial: func(_, _ string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			}

			reqBytes, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			resp, err := client.Post("http://unix/call-once", "application/json", bytes.NewBuffer(reqBytes))
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
	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")

	return cmd
}
