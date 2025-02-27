package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/spf13/cobra"
)

// PsCmd creates a new cobra command for listing running functions
var PsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running functions",
	Long:  "List all running functions and their status.",
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
				ui.PrintInfo("Warning", "Engine is not running. No functions will be shown.")
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

		// Define styles for pretty formatting
		tableHeaderStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ui.InfoColor)).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(ui.DimTextColor)).
			BorderBottom(true)

		tableRowStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			PaddingLeft(1)

		// Prepare table rows
		var tableRows []string
		
		// Add header row
		tableRows = append(tableRows, tableHeaderStyle.Render(fmt.Sprintf(" %-20s %-20s %-15s",
			"NAMESPACE", "NAME", "STATUS")))
		
		// Add rows for all running functions
		if engineRunning && len(runningFunctions) > 0 {
			for _, fn := range runningFunctions {
				statusText := lipgloss.NewStyle().
					Foreground(lipgloss.Color(ui.SuccessColor)).
					Render("running")
					
				row := tableRowStyle.Render(fmt.Sprintf("%-20s %-20s %-15s",
					fn.Namespace, fn.Name, statusText))
				tableRows = append(tableRows, row)
			}
		}

		// Combine rows into a table for pretty output
		if len(tableRows) > 1 {
			table := lipgloss.JoinVertical(lipgloss.Left, tableRows...)
			output := lipgloss.JoinVertical(lipgloss.Left, "\n", table, "\n")
			fmt.Println(output)
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