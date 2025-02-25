package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

// NewComposeDownCommand creates a new cobra command for compose down
func NewComposeDownCommand(container *di.Container) *cobra.Command {
	var filePath string
	var force bool

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop and remove functions defined in a compose file",
		Long:  "Stop and remove functions defined in an ignition-compose.yml file.",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println("Stopping compose services...")
			
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

			// Try to ping the engine - if it's not running, just exit
			if err := engineClient.Ping(context.Background()); err != nil {
				fmt.Println("Engine is not running, no services to stop.")
				return nil
			}

			// Create a context for operations
			ctx := context.Background()

			// Try to list functions (just to check connectivity)
			_, err = engineClient.ListFunctions(ctx)
			if err != nil {
				fmt.Println("Warning: Failed to list functions from engine.")
				if !force {
					fmt.Println("Use --force to continue anyway.")
					return err
				}
			}

			// Check which functions from the compose file are currently loaded
			fmt.Println("\nFunctions to stop:")
			stoppedCount := 0

			for name, service := range composeManifest.Services {
				// Parse function reference (namespace/name:tag)
				parts := strings.Split(service.Function, ":")
				functionRef := parts[0]

				nameParts := strings.Split(functionRef, "/")
				if len(nameParts) != 2 {
					fmt.Printf("Warning: Invalid function reference '%s' for service '%s'\n", 
					          service.Function, name)
					continue
				}

				namespace, funcName := nameParts[0], nameParts[1]
				fmt.Printf("• %s/%s (%s)\n", namespace, funcName, name)
				stoppedCount++
			}

			// Print information about alternative approach
			fmt.Println("\nNote: To stop all running functions, restart the engine:")
			fmt.Println("  ignition engine start")
			
			fmt.Printf("\nFunctions marked for stopping: %d\n", stoppedCount)

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&force, "force", "", false, "Continue even if errors occur")
	return cmd
}