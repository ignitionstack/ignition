package compose

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

// NewComposeDownCommand creates a new cobra command for compose down
func NewComposeDownCommand(container *di.Container) *cobra.Command {
	var filePath string

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

			// Create a context for the unloading operations
			ctx := context.Background()

			// Initialize error tracking
			var unloadErrs []string
			var unloadErrsMu sync.Mutex
			var wg sync.WaitGroup

			// Unload each function in the compose file
			for name, service := range composeManifest.Services {
				wg.Add(1)
				go func(name string, service manifest.ComposeService) {
					defer wg.Done()

					// Parse function reference (namespace/name:tag)
					parts := strings.Split(service.Function, ":")
					functionRef := parts[0]

					nameParts := strings.Split(functionRef, "/")
					if len(nameParts) != 2 {
						unloadErrsMu.Lock()
						unloadErrs = append(unloadErrs, fmt.Sprintf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name))
						unloadErrsMu.Unlock()
						return
					}

					namespace, funcName := nameParts[0], nameParts[1]
						
					// Unload the function from the engine
					fmt.Printf("Stopping function: %s/%s\n", namespace, funcName)
					
					// Attempt to unload the function
					err := engineClient.UnloadFunction(ctx, namespace, funcName)
					if err != nil {
						unloadErrsMu.Lock()
						unloadErrs = append(unloadErrs, fmt.Sprintf("failed to unload function '%s' for service '%s': %v", service.Function, name, err))
						unloadErrsMu.Unlock()
						return
					}
				}(name, service)
			}

			// Wait for all functions to unload
			wg.Wait()

			// Check for errors
			if len(unloadErrs) > 0 {
				return fmt.Errorf("failed to unload some functions:\n%s", strings.Join(unloadErrs, "\n"))
			}

			// Print success message
			fmt.Printf("Successfully stopped %d services\n", len(composeManifest.Services))

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	return cmd
}