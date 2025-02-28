package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// NewComposeInitCommand creates a new cobra command for compose init
func NewComposeInitCommand(container *di.Container) *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new compose file",
		Long:  "Create a new ignition-compose.yml file with example services.",
		RunE: func(c *cobra.Command, args []string) error {
			// Removed redundant operation line

			// Create example compose file content
			example := map[string]interface{}{
				"version": "1",
				"services": map[string]interface{}{
					"api": map[string]interface{}{
						"function": "my_namespace/api_service:latest",
						"config": map[string]string{
							"DEBUG": "true",
						},
					},
					"processor": map[string]interface{}{
						"function":   "my_namespace/processor:v1.2.0",
						"depends_on": []string{"api"},
					},
					"worker": map[string]interface{}{
						"function": "my_namespace/worker:latest",
						"restart":  "always",
					},
				},
			}

			// Marshal to YAML
			yamlData, err := yaml.Marshal(example)
			if err != nil {
				return fmt.Errorf("failed to generate YAML: %w", err)
			}

			// If no output path specified, use default
			if outputPath == "" {
				outputPath = "ignition-compose.yml"
			}

			// Make sure directory exists
			dir := filepath.Dir(outputPath)
			if dir != "." {
				if err := os.MkdirAll(dir, 0755); err != nil {
					ui.PrintError(fmt.Sprintf("Failed to create directory %s: %v", dir, err))
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Check if file already exists
			if _, err := os.Stat(outputPath); err == nil {
				ui.PrintError(fmt.Sprintf("File %s already exists", outputPath))
				return fmt.Errorf("file %s already exists. Use -o to specify a different output path", outputPath)
			}

			// Write to file
			if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
				ui.PrintError(fmt.Sprintf("Failed to write file: %v", err))
				return fmt.Errorf("failed to write file: %w", err)
			}

			ui.PrintSuccess(fmt.Sprintf("Created compose file: %s", outputPath))

			ui.PrintMetadata("Usage", "To start these services, update the function references and run:")
			ui.PrintHighlight("  ignition compose up")

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: ignition-compose.yml)")
	return cmd
}
