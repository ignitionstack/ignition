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
	var dryRun bool

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

			// Check if engine is running
			if err := engineClient.Ping(context.Background()); err != nil {
				fmt.Println("Engine is not running, no services to stop.")
				return nil
			}

			// Show functions that will be stopped
			fmt.Println("\nFunctions to stop:")
			count := 0
			var functionsToUnload []struct {
				namespace string
				name      string
				service   string
			}
			
			for name, service := range composeManifest.Services {
				parts := strings.Split(service.Function, ":")
				functionRef := parts[0]
				nameParts := strings.Split(functionRef, "/")
				if len(nameParts) != 2 {
					fmt.Printf("Warning: Invalid function reference %s\n", service.Function)
					continue
				}
				namespace, funcName := nameParts[0], nameParts[1]
				fmt.Printf("• %s/%s (%s)\n", namespace, funcName, name)
				count++
				
				functionsToUnload = append(functionsToUnload, struct {
					namespace string
					name      string
					service   string
				}{
					namespace: namespace,
					name:      funcName,
					service:   name,
				})
			}

			// If dry run, just show what would be unloaded
			if dryRun {
				fmt.Println("\nNote: This is a dry run. Functions are still running!")
				fmt.Println("To stop all functions, run without --dry-run")
				fmt.Printf("\nFunctions marked for stopping: %d\n", count)
				return nil
			}
			
			// Unload functions concurrently
			if count > 0 {
				fmt.Println("\nUnloading functions...")
				
				var unloadErrs []string
				var unloadErrsMu sync.Mutex
				var wg sync.WaitGroup
				ctx := context.Background()
				
				for _, function := range functionsToUnload {
					wg.Add(1)
					go func(namespace, name, serviceName string) {
						defer wg.Done()
						
						fmt.Printf("Unloading function: %s/%s\n", namespace, name)
						err := engineClient.UnloadFunction(ctx, namespace, name)
						if err != nil {
							unloadErrsMu.Lock()
							unloadErrs = append(unloadErrs, fmt.Sprintf("failed to unload function '%s/%s' for service '%s': %v", 
								namespace, name, serviceName, err))
							unloadErrsMu.Unlock()
						}
					}(function.namespace, function.name, function.service)
				}
				
				// Wait for all unload operations to complete
				wg.Wait()
				
				// Check for errors
				if len(unloadErrs) > 0 {
					return fmt.Errorf("failed to unload some functions:\n%s", strings.Join(unloadErrs, "\n"))
				}
				
				fmt.Printf("\nSuccessfully stopped %d functions\n", count)
			} else {
				fmt.Println("\nNo functions to unload")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be unloaded without actually unloading")
	return cmd
}