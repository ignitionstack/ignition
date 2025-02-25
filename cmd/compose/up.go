package compose

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

// NewComposeUpCommand creates a new cobra command for compose up
func NewComposeUpCommand(container *di.Container) *cobra.Command {
	var filePath string
	var detach bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Create and start functions defined in a compose file",
		Long:  "Create and start functions defined in an ignition-compose.yml file.",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println("Starting compose services...")
			
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

			// Ping the engine to ensure it's running
			if err := engineClient.Ping(context.Background()); err != nil {
				return fmt.Errorf("engine is not running. Start it with 'ignition engine start': %w", err)
			}

			// Create a context for the loading operations
			ctx := context.Background()
			
			// Print loading message
			fmt.Println("Loading functions from compose file...")

			// Initialize error tracking
			var loadErrs []string
			var loadErrsMu sync.Mutex
			var wg sync.WaitGroup

			// Load each function in the compose file
			for name, service := range composeManifest.Services {
				wg.Add(1)
				go func(name string, service manifest.ComposeService) {
					defer wg.Done()

					// Parse function reference (namespace/name:tag)
					parts := strings.Split(service.Function, ":")
					functionRef := parts[0]
					tag := "latest"
					if len(parts) > 1 {
						tag = parts[1]
					}

					nameParts := strings.Split(functionRef, "/")
					if len(nameParts) != 2 {
						loadErrsMu.Lock()
						loadErrs = append(loadErrs, fmt.Sprintf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name))
						loadErrsMu.Unlock()
						return
					}

					namespace, funcName := nameParts[0], nameParts[1]

					// Load the function into the engine
					fmt.Printf("Loading function: %s/%s:%s\n", namespace, funcName, tag)
					err := engineClient.LoadFunction(ctx, namespace, funcName, tag)
					if err != nil {
						loadErrsMu.Lock()
						loadErrs = append(loadErrs, fmt.Sprintf("failed to load function '%s' for service '%s': %v", service.Function, name, err))
						loadErrsMu.Unlock()
						return
					}
				}(name, service)
			}

			// Wait for all functions to load
			wg.Wait()

			// Check for errors
			if len(loadErrs) > 0 {
				return fmt.Errorf("failed to load some functions:\n%s", strings.Join(loadErrs, "\n"))
			}

			// Print success message
			fmt.Printf("Successfully started %d functions from compose file\n", len(composeManifest.Services))
			
			// List running functions
			fmt.Println("\nRunning functions:")
			for name, service := range composeManifest.Services {
				fmt.Printf("  • %s (%s)\n", name, service.Function)
			}

			// If not detached, keep the process running until interrupted
			if !detach {
				fmt.Println("\nFunctions are running. Press Ctrl+C to stop...")
				
				// Set up signal handling for graceful shutdown
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
				
				// Block until we receive a signal
				<-sigChan
				
				fmt.Println("\nShutting down functions...")
				
				// Implement graceful shutdown here if needed
				// For now, just inform the user they need to run "compose down"
				fmt.Println("Run 'ignition compose down' to stop all services")
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run in detached mode")
	
	return cmd
}