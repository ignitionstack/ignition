package compose

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

// NewComposePsCommand creates a new cobra command for compose ps
func NewComposePsCommand(container *di.Container) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List functions loaded from a compose file",
		Long:  "List functions defined in an ignition-compose.yml file and their current status.",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println("Listing compose services...")
			
			// Parse the compose file
			composeManifest, err := manifest.ParseComposeFile(filePath)
			if err != nil {
				return err
			}

			// Get the engine client from the container
			client, err := container.Get("engineClient")
			if err != nil {
				return fmt.Errorf("error getting engine client: %w", err)
			}
			engineClient, ok := client.(*services.EngineClient)
			if !ok {
				return fmt.Errorf("invalid engine client type")
			}

			// Try to ping the engine to ensure it's running
			ctx := context.Background()
			engineRunning := true
			if err := engineClient.Ping(ctx); err != nil {
				engineRunning = false
				fmt.Println("Warning: Engine is not running. Function status may not be accurate.")
			}

			// Get all loaded functions if engine is running
			var loadedFunctions []services.FunctionDetails
			if engineRunning {
				loadedFunctions, err = engineClient.ListFunctions(ctx)
				if err != nil {
					fmt.Println("Warning: Failed to list functions from engine. Status information may not be accurate.")
				}
			}

			// Create a writer for tabular output
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "SERVICE\tFUNCTION\tSTATUS")

			// Check each function in the compose file
			for name, service := range composeManifest.Services {
				// Parse function reference (namespace/name:tag)
				parts := strings.Split(service.Function, ":")
				functionRef := parts[0]
				
				nameParts := strings.Split(functionRef, "/")
				if len(nameParts) != 2 {
					return fmt.Errorf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name)
				}

				namespace, funcName := nameParts[0], nameParts[1]
				status := "stopped"

				// Check if function is loaded
				if engineRunning {
					for _, fn := range loadedFunctions {
						if fn.Namespace == namespace && fn.Name == funcName {
							status = "running"
							break
						}
					}
				}

				fmt.Fprintf(w, "%s\t%s\t%s\n", name, service.Function, status)
			}

			// Flush the writer to print the table
			w.Flush()

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	return cmd
}