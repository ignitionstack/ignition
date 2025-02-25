package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/internal/di"
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
			fmt.Println("Creating example compose file...")
			
			// Create example compose file content
			example := map[string]interface{}{
				"version": "1",
				"services": map[string]interface{}{
					"api": map[string]interface{}{
						"function":    "my_namespace/api_service:latest",
						"environment": map[string]string{
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
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Check if file already exists
			if _, err := os.Stat(outputPath); err == nil {
				return fmt.Errorf("file %s already exists. Use -o to specify a different output path", outputPath)
			}

			// Write to file
			if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Created compose file: %s\n", outputPath)
			fmt.Println("\nTo start these services, update the function references and run:")
			fmt.Println("  ignition compose up")

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: ignition-compose.yml)")
	return cmd
}