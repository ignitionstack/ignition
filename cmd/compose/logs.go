package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

// NewComposeLogsCommand creates a new cobra command for retrieving and displaying function logs
func NewComposeLogsCommand(container *di.Container) *cobra.Command {
	var filePath string
	var follow bool
	var since string
	var tail int

	cmd := &cobra.Command{
		Use:   "logs [SERVICE...]",
		Short: "View logs from services defined in a compose file",
		Long:  "View the logs from services defined in an ignition-compose.yml file.",
		RunE: func(c *cobra.Command, args []string) error {
			ui.PrintInfo("Operation", "Viewing service logs")
			
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

			// Ping the engine to ensure it's running
			if err := engineClient.Ping(context.Background()); err != nil {
				ui.PrintError("Engine is not running")
				return fmt.Errorf("engine is not running. Start it with 'ignition engine start': %w", err)
			}

			// Filter services based on args
			servicesToShow := make(map[string]manifest.ComposeService)
			if len(args) > 0 {
				// Show only the specified services
				for _, serviceName := range args {
					if service, exists := composeManifest.Services[serviceName]; exists {
						servicesToShow[serviceName] = service
					} else {
						ui.PrintWarning(fmt.Sprintf("Service '%s' not found in compose file", serviceName))
					}
				}
				if len(servicesToShow) == 0 {
					ui.PrintError("None of the specified services were found in the compose file")
					return fmt.Errorf("no matching services found")
				}
			} else {
				// Show all services
				servicesToShow = composeManifest.Services
			}

			// Convert the since string to a time.Duration
			var sinceDuration time.Duration
			if since != "" {
				sinceDuration, err = time.ParseDuration(since)
				if err != nil {
					ui.PrintError(fmt.Sprintf("Invalid time duration '%s': %v", since, err))
					return err
				}
			}

			// Create a spinner for retrieving logs
			spinnerModel := spinner.NewSpinnerModelWithMessage("Retrieving logs from services...")
			program := tea.NewProgram(spinnerModel)
			
			// Retrieve logs in a goroutine
			go func() {
				logs, err := retrieveLogs(context.Background(), servicesToShow, engineClient, sinceDuration, tail)
				if err != nil {
					program.Send(spinner.ErrorMsg{Err: err})
				} else {
					program.Send(spinner.DoneMsg{Result: logs})
				}
			}()
			
			// Run the spinner UI
			model, err := program.Run()
			if err != nil {
				ui.PrintError(fmt.Sprintf("UI error: %v", err))
				return err
			}
			
			// Check for errors during log retrieval
			finalModel := model.(spinner.SpinnerModel)
			if finalModel.HasError() {
				return finalModel.GetError()
			}
			
			// Display the logs
			logs := finalModel.GetResult().(map[string][]string)
			displayLogs(logs)

			// If follow mode is enabled, continuously retrieve and display new logs
			if follow {
				ui.PrintInfo("Status", "Watching for new logs. Press Ctrl+C to stop...")
				
				// Get the most recent timestamp to filter future logs
				lastSeen := time.Now()
				
				// Setup a ticker for polling
				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()
				
				// Create context for cancellation
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				
				// Start polling for new logs
				for {
					select {
					case <-ticker.C:
						// Retrieve logs since lastSeen time
						newLogs, err := retrieveLogs(ctx, servicesToShow, engineClient, time.Since(lastSeen), 0)
						if err != nil {
							ui.PrintError(fmt.Sprintf("Error retrieving new logs: %v", err))
							return err
						}
						
						// Display only new logs
						if hasNewLogs(newLogs) {
							displayLogs(newLogs)
							lastSeen = time.Now()
						}
						
					case <-ctx.Done():
						return nil
					}
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&follow, "follow", "F", false, "Follow log output")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since timestamp (e.g., 30m for 30 minutes)")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show from the end of the logs")
	
	return cmd
}

// retrieveLogs gets logs for the specified services
func retrieveLogs(ctx context.Context, services map[string]manifest.ComposeService, client *services.EngineClient, since time.Duration, tail int) (map[string][]string, error) {
	logs := make(map[string][]string)
	
	for name, service := range services {
		// Parse function reference (namespace/name:tag)
		parts := strings.Split(service.Function, ":")
		functionRef := parts[0]
		
		nameParts := strings.Split(functionRef, "/")
		if len(nameParts) != 2 {
			return nil, fmt.Errorf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name)
		}
		
		namespace, funcName := nameParts[0], nameParts[1]
		
		// Retrieve logs for the function
		functionLogs, err := client.GetFunctionLogs(ctx, namespace, funcName, since, tail)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve logs for service '%s': %v", name, err)
		}
		
		logs[name] = functionLogs
	}
	
	return logs, nil
}

// displayLogs formats and displays logs with service name prefixes
func displayLogs(logs map[string][]string) {
	for serviceName, serviceLog := range logs {
		if len(serviceLog) == 0 {
			continue
		}
		
		for _, line := range serviceLog {
			// Format each log line with service name prefix
			fmt.Printf("%s | %s\n", ui.StyleServiceName(serviceName), line)
		}
	}
}

// hasNewLogs checks if there are any new log lines to display
func hasNewLogs(logs map[string][]string) bool {
	for _, logLines := range logs {
		if len(logLines) > 0 {
			return true
		}
	}
	return false
}