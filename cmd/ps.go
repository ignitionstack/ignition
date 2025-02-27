package cmd

import (
	"context"
	"fmt"

	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/spf13/cobra"
)

// PsCmd creates a new cobra command for listing running functions
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
	RunE: func(c *cobra.Command, args []string) error {
		// Check if output should be machine-readable
		plainFormat, _ := c.Flags().GetBool("plain")

		if !plainFormat {
			ui.PrintInfo("Operation", "Listing running functions")
		}

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
			return fmt.Errorf("invalid engine client type")
		}

		// Try to ping the engine to ensure it's running
		ctx := context.Background()
		engineRunning := true
		if err := engineClient.Ping(ctx); err != nil {
			engineRunning = false
			if !plainFormat {
				ui.PrintWarning("Engine is not running. No functions will be shown.")
			}
		}

		// Get all loaded functions if engine is running
		var runningFunctions []services.EngineFunctionDetails
		if engineRunning {
			runningFunctions, err = engineClient.ListFunctions(ctx)
			if err != nil {
				if !plainFormat {
					ui.PrintError(fmt.Sprintf("Failed to list functions: %v", err))
				}
				return fmt.Errorf("failed to list functions: %w", err)
			}
		}

		// Output in machine-readable format if required
		if plainFormat {
			// Define format strings with exact field widths
			const headerFormat = "%-20s\t%-20s\t%-15s\n"
			const dataFormat = "%-20s\t%-20s\t%-15s\n"

			// Print header
			fmt.Printf(headerFormat, "NAMESPACE", "NAME", "STATUS")

			// Print all running functions
			if engineRunning && len(runningFunctions) > 0 {
				for _, fn := range runningFunctions {
					fmt.Printf(dataFormat, fn.Namespace, fn.Name, "running")
				}
			} else {
				fmt.Println("No running functions")
			}
			return nil
		}

		// Create a table using the centralized table component
		table := ui.NewTable([]string{"NAMESPACE", "NAME", "STATUS"})

		// Add rows for all running functions
		if engineRunning && len(runningFunctions) > 0 {
			for _, fn := range runningFunctions {
				table.AddRow(fn.Namespace, fn.Name, ui.StyleStatusValue("running"))
			}

			// Render the table
			fmt.Println(ui.RenderTable(table))
		} else {
			ui.PrintInfo("Status", "No running functions")
		}

		return nil
	},
}

func init() {
	PsCmd.Flags().Bool("plain", false, "Output in plain, machine-readable format (useful for piping to other commands)")
	rootCmd.AddCommand(PsCmd)
}
