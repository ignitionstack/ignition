package compose

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
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
			ui.PrintInfo("Operation", "Stopping compose services")
			
			// Parse the compose file
			composeManifest, err := manifest.ParseComposeFile(filePath)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to parse compose file: %v", err))
				return err
			}

			// Get the engine client from the container
			client, err := container.Get("engineClient")
			if err != nil {
				ui.PrintError("Error getting engine client")
				return fmt.Errorf("error getting engine client: %w", err)
			}
			engineClient, ok := client.(*services.EngineClient)
			if !ok {
				ui.PrintError("Invalid engine client type")
				return fmt.Errorf("invalid engine client type")
			}

			// Check if engine is running
			if err := engineClient.Ping(context.Background()); err != nil {
				ui.PrintInfo("Status", "Engine is not running, no services to stop")
				return nil
			}

			// Gather functions to unload
			ui.PrintInfo("Status", "Functions to stop")
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
					ui.PrintError(fmt.Sprintf("Invalid function reference %s", service.Function))
					continue
				}
				namespace, funcName := nameParts[0], nameParts[1]
				ui.PrintMetadata(name, fmt.Sprintf("%s/%s", namespace, funcName))
				
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
				ui.PrintInfo("Status", "This is a dry run - no functions were stopped")
				ui.PrintMetadata("Action", "Run without --dry-run to actually stop functions")
				ui.PrintSuccess(fmt.Sprintf("Would stop %d functions", len(functionsToUnload)))
				return nil
			}
			
			// If no functions to unload, exit early
			if len(functionsToUnload) == 0 {
				ui.PrintInfo("Status", "No functions to unload")
				return nil
			}
			
			// Create a spinner for the unloading process
			spinnerModel := spinner.NewSpinnerModelWithMessage("Unloading functions...")
			program := tea.NewProgram(spinnerModel)
			
			// Unload functions in a goroutine
			go func() {
				err := unloadFunctions(context.Background(), functionsToUnload, engineClient)
				if err != nil {
					program.Send(spinner.ErrorMsg{Err: err})
				} else {
					program.Send(spinner.DoneMsg{Result: len(functionsToUnload)})
				}
			}()
			
			// Run the spinner UI
			model, err := program.Run()
			if err != nil {
				ui.PrintError(fmt.Sprintf("UI error: %v", err))
				return err
			}
			
			// Check for errors during unloading
			finalModel := model.(spinner.SpinnerModel)
			if finalModel.HasError() {
				return finalModel.GetError()
			}
			
			// Print success message
			ui.PrintSuccess(fmt.Sprintf("Successfully stopped %d functions", len(functionsToUnload)))
			
			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be unloaded without actually unloading")
	return cmd
}

// unloadFunctions unloads all functions in the provided list
func unloadFunctions(ctx context.Context, functions []struct {
	namespace string
	name      string
	service   string
}, engineClient *services.EngineClient) error {
	var unloadErrs []string
	var unloadErrsMu sync.Mutex
	var wg sync.WaitGroup
	
	for _, function := range functions {
		wg.Add(1)
		go func(namespace, name, serviceName string) {
			defer wg.Done()
			
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
	
	return nil
}