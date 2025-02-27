package function

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/spf13/cobra"
)

func NewFunctionListCommand() *cobra.Command {
	var socketPath string
	cmd := &cobra.Command{
		Use:     "list [namespace/name]",
		Aliases: []string{"ls"},
		Short:   "List functions in the registry",
		Long: `Display all functions registered in the Ignition function registry.

If called without arguments, lists all available functions in the registry.
If a namespace/name is provided, shows detailed information about all versions
of that specific function including:
* Repository name (namespace/name)
* Tags
* Function ID (hash)
* Size

The registry contains all functions that have been built or loaded, and this
command allows you to explore what's available to run.`,
		Example: `  # List all available functions
  ignition function list

  # List all versions of a specific function
  ignition function list my-namespace/my-function
  
  # List in plain format (useful for scripting)
  ignition function list --plain`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if output should be machine-readable
			plainFormat, _ := cmd.Flags().GetBool("plain")

			var req map[string]string

			if len(args) == 1 {
				namespace, name, err := parseNamespaceAndNameWithoutTag(args[0])
				if err != nil {
					return fmt.Errorf("invalid function name format: %w", err)
				}

				req = map[string]string{
					"namespace": namespace,
					"name":      name,
				}
			} else {
				req = map[string]string{}
			}

			reqBody, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to encode request: %w", err)
			}

			client := http.Client{
				Transport: &http.Transport{
					Dial: func(_, _ string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			}

			resp, err := client.Post("http://unix/list", "application/json", bytes.NewBuffer(reqBody))
			if err != nil {
				return fmt.Errorf("failed to send request to engine: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("engine returned error: %s", resp.Status)
			}

			if len(args) == 1 {
				var metadata registry.FunctionMetadata
				if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
					return fmt.Errorf("failed to decode response: %w", err)
				}

				if plainFormat {
					renderFunctionMetadataPlain(metadata)
				} else {
					renderFunctionMetadata(metadata)
				}
			} else {
				var metadataList []registry.FunctionMetadata
				if err := json.NewDecoder(resp.Body).Decode(&metadataList); err != nil {
					return fmt.Errorf("failed to decode response: %w", err)
				}

				if plainFormat {
					renderFunctionListPlain(metadataList)
				} else {
					renderFunctionList(metadataList)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().Bool("plain", false, "Output in plain, machine-readable format (useful for piping to other commands)")
	return cmd
}

func parseNamespaceAndNameWithoutTag(input string) (namespace, name string, err error) {
	parts := strings.Split(input, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid format: expected namespace/name")
	}

	namespace = strings.TrimSpace(parts[0])
	name = strings.TrimSpace(parts[1])

	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("namespace and name cannot be empty")
	}

	return namespace, name, nil
}

func renderFunctionListPlain(metadataList []registry.FunctionMetadata) {
	// Use format strings with exact field widths for consistent alignment
	const headerFormat = "%-30s\t%-15s\t%-20s\t%-10s\n"
	const dataFormat = "%-30s\t%-15s\t%-20s\t%-10s\n"

	// Print header with exact spacing
	fmt.Printf(headerFormat, "REPOSITORY", "TAG", "FUNCTION_ID", "SIZE")

	// Print each function version with consistent alignment
	for _, metadata := range metadataList {
		for _, version := range metadata.Versions {
			repository := fmt.Sprintf("%s/%s", metadata.Namespace, metadata.Name)

			if len(version.Tags) == 0 {
				fmt.Printf(dataFormat,
					repository,
					"<none>",
					version.Hash,
					formatSize(version.Size))
			} else {
				sortedTags := make([]string, len(version.Tags))
				copy(sortedTags, version.Tags)
				sort.Strings(sortedTags)

				for _, tag := range sortedTags {
					fmt.Printf(dataFormat,
						repository,
						tag,
						version.Hash,
						formatSize(version.Size))
				}
			}
		}
	}
}

func renderFunctionMetadataPlain(metadata registry.FunctionMetadata) {
	// Use format strings with exact field widths for consistent alignment
	const headerFormat = "%-30s\t%-15s\t%-20s\t%-10s\n"
	const dataFormat = "%-30s\t%-15s\t%-20s\t%-10s\n"

	// Print header with exact spacing
	fmt.Printf(headerFormat, "REPOSITORY", "TAG", "FUNCTION_ID", "SIZE")

	// Print each version with consistent alignment
	for _, version := range metadata.Versions {
		repository := fmt.Sprintf("%s/%s", metadata.Namespace, metadata.Name)

		if len(version.Tags) == 0 {
			fmt.Printf(dataFormat,
				repository,
				"<none>",
				version.Hash,
				formatSize(version.Size))
		} else {
			sortedTags := make([]string, len(version.Tags))
			copy(sortedTags, version.Tags)
			sort.Strings(sortedTags)

			for _, tag := range sortedTags {
				fmt.Printf(dataFormat,
					repository,
					tag,
					version.Hash,
					formatSize(version.Size))
			}
		}
	}
}

func renderFunctionList(metadataList []registry.FunctionMetadata) {
	table := ui.NewTable([]string{"REPOSITORY", "TAG", "FUNCTION ID", "SIZE"})

	for _, metadata := range metadataList {
		for _, version := range metadata.Versions {
			repository := fmt.Sprintf("%s/%s", metadata.Namespace, metadata.Name)

			if len(version.Tags) == 0 {
				table.AddRow(repository, "<none>", version.Hash, formatSize(version.Size))
			} else {
				sortedTags := make([]string, len(version.Tags))
				copy(sortedTags, version.Tags)
				sort.Strings(sortedTags)

				for _, tag := range sortedTags {
					table.AddRow(repository, tag, version.Hash, formatSize(version.Size))
				}
			}
		}
	}

	// Render the table
	fmt.Println(ui.RenderTable(table))
}

func renderFunctionMetadata(metadata registry.FunctionMetadata) {
	table := ui.NewTable([]string{"REPOSITORY", "TAG", "FUNCTION ID", "SIZE"})

	for _, version := range metadata.Versions {
		repository := fmt.Sprintf("%s/%s", metadata.Namespace, metadata.Name)

		if len(version.Tags) == 0 {
			table.AddRow(repository, "<none>", version.Hash, formatSize(version.Size))
		} else {
			sortedTags := make([]string, len(version.Tags))
			copy(sortedTags, version.Tags)
			sort.Strings(sortedTags)

			for _, tag := range sortedTags {
				table.AddRow(repository, tag, version.Hash, formatSize(version.Size))
			}
		}
	}

	// Render the table
	fmt.Println(ui.RenderTable(table))
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
