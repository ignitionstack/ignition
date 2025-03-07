package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/pkg/types"
	"github.com/spf13/cobra"
)

const (
	StatusStopped  = "stopped"
	StatusUnloaded = "unloaded"
	StatusRunning  = "running"
)

// PsCmd creates a new cobra command for listing running functions.
var PsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running functions",
	Long: `List all running functions currently loaded in the Ignition engine.

This command connects to the running engine and displays details about all
currently loaded and running WebAssembly functions, including:
* Namespace
* Function name
* Running status

The command requires that the Ignition engine is already running. If the engine
is not running, it will display a warning and show no functions.`,
	Example: `  # List all running functions
  ignition ps

  # List in plain format (useful for scripting)
  ignition ps --plain`,
	RunE: func(c *cobra.Command, _ []string) error {
		// Check if output should be machine-readable
		plainFormat, _ := c.Flags().GetBool("plain")

		// Get the engine client from the container
		client, err := Container.Get("engineClient")
		if err != nil {
			if !plainFormat {
				ui.PrintError("Error getting engine client")
			}
			return fmt.Errorf("error getting engine client: %w", err)
		}
		engineClient, ok := client.(*services.EngineClient)
		if !ok {
			if !plainFormat {
				ui.PrintError("Invalid engine client type")
			}
			return errors.New("invalid engine client type")
		}

		// Try to ping the engine to ensure it's running
		ctx := context.Background()
		engineRunning := true
		if err := engineClient.Status(ctx); err != nil {
			engineRunning = false
			if !plainFormat {
				ui.PrintWarning("Engine is not running. No functions will be shown.")
			}
		}

		// Get all loaded functions if engine is running
		var runningFunctions []types.LoadedFunction
		if engineRunning {
			loadedFunctions, err := engineClient.ListFunctions(ctx)
			if err != nil {
				if !plainFormat {
					ui.PrintError(fmt.Sprintf("Failed to list functions: %v", err))
				}
				return fmt.Errorf("failed to list functions: %w", err)
			}
			runningFunctions = loadedFunctions
		}

		// Output in machine-readable format if required
		if plainFormat {
			// Define format strings with exact field widths
			const headerFormat = "%-20s\t%-20s\t%-15s\n"
			const dataFormat = "%-20s\t%-20s\t%-15s\n"

			// Print header
			fmt.Printf(headerFormat, "NAMESPACE", "NAME", "STATUS")

			// Print all functions
			if engineRunning && len(runningFunctions) > 0 {
				for _, fn := range runningFunctions {
					status := fn.Status
					if status == "" {
						status = StatusRunning
					} else if status == StatusStopped {
						status = StatusStopped
					} else if status == StatusUnloaded {
						status = StatusUnloaded
					}
					fmt.Printf(dataFormat, fn.Namespace, fn.Name, status)
				}
			} else {
				fmt.Println("No functions found")
			}
			return nil
		}

		// Create a table using the centralized table component
		table := ui.NewTable([]string{"NAMESPACE", "NAME", "STATUS"})

		// Add rows for all functions
		if engineRunning && len(runningFunctions) > 0 {
			unloadedFunctionsExist := false
			stoppedFunctionsExist := false

			for _, fn := range runningFunctions {
				var statusStyle string

				if fn.Status == StatusUnloaded {
					statusStyle = ui.StyleStatusValue(StatusUnloaded)
					unloadedFunctionsExist = true
				} else if fn.Status == StatusStopped {
					statusStyle = ui.StyleStatusValue(StatusStopped)
					stoppedFunctionsExist = true
				} else {
					statusStyle = ui.StyleStatusValue(StatusRunning)
				}

				table.AddRow(fn.Namespace, fn.Name, statusStyle)
			}

			// Render the table
			fmt.Println(ui.RenderTable(table))

			// Show explanation notes for different function statuses
			if unloadedFunctionsExist || stoppedFunctionsExist {
				fmt.Println()

				if unloadedFunctionsExist {
					ui.PrintInfo("Note", "Functions with '"+StatusUnloaded+"' status are available but not currently loaded in memory")
				}

				if stoppedFunctionsExist {
					ui.PrintInfo("Note", "Functions with '"+StatusStopped+"' status will not be automatically reloaded when called")
				}
			}
		} else {
			ui.PrintInfo("Status", "No functions found")
		}

		return nil
	},
}

func init() {
	PsCmd.Flags().Bool("plain", false, "Output in plain, machine-readable format (useful for piping to other commands)")
	rootCmd.AddCommand(PsCmd)
}
