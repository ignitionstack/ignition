package compose

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
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
			ui.PrintInfo("Operation", "Listing compose services")
			
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

			// Try to ping the engine to ensure it's running
			ctx := context.Background()
			engineRunning := true
			if err := engineClient.Ping(ctx); err != nil {
				engineRunning = false
				ui.PrintInfo("Warning", "Engine is not running. Function status may not be accurate")
			}

			// Get all loaded functions if engine is running
			var loadedFunctions []services.EngineFunctionDetails
			if engineRunning {
				loadedFunctions, err = engineClient.ListFunctions(ctx)
				if err != nil {
					ui.PrintInfo("Warning", "Failed to list functions from engine. Status information may not be accurate")
				}
			}

			// Create a tabwriter with appropriate spacing
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

			// Define styles
			headerStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(ui.AccentColor))
			
			runningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ui.SuccessColor))
			stoppedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ui.DimTextColor))

			// First, format all cells to ensure consistent styling
			service := headerStyle.Render("SERVICE")
			function := headerStyle.Render("FUNCTION")
			status := headerStyle.Render("STATUS")

			// Print header
			fmt.Fprintf(w, "%s\t%s\t%s\n", service, function, status)

			// Create a map of loaded functions for efficient lookup
			loadedFunctionsMap := make(map[string]bool)
			if engineRunning {
				for _, fn := range loadedFunctions {
					key := fmt.Sprintf("%s/%s", fn.Namespace, fn.Name)
					loadedFunctionsMap[key] = true
				}
			}

			// Check each function in the compose file
			for name, service := range composeManifest.Services {
				// Parse function reference (namespace/name:tag)
				parts := strings.Split(service.Function, ":")
				functionRef := parts[0]
				
				nameParts := strings.Split(functionRef, "/")
				if len(nameParts) != 2 {
					ui.PrintError(fmt.Sprintf("Invalid function reference '%s' for service '%s'", service.Function, name))
					continue
				}

				namespace, funcName := nameParts[0], nameParts[1]
				
				// Determine if the function is loaded
				isLoaded := false
				if engineRunning {
					key := fmt.Sprintf("%s/%s", namespace, funcName)
					isLoaded = loadedFunctionsMap[key]
				}
				
				// Format status with color
				statusText := "stopped"
				if isLoaded {
					statusText = runningStyle.Render("running")
				} else {
					statusText = stoppedStyle.Render("stopped")
				}

				// Print row
				fmt.Fprintf(w, "%s\t%s\t%s\n", name, service.Function, statusText)
			}

			// Flush the table writer
			w.Flush()
			
			if len(composeManifest.Services) == 0 {
				ui.PrintInfo("Status", "No services defined in the compose file")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	return cmd
}