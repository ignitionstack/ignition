package compose

import (
	"context"
	"errors"
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

// NewComposeLogsCommand creates a new cobra command for retrieving and displaying function logs.
func NewComposeLogsCommand(container *di.Container) *cobra.Command {
	var filePath string
	var follow bool
	var since string
	var tail int

	cmd := &cobra.Command{
		Use:   "logs [SERVICE...]",
		Short: "View logs from services defined in a compose file",
		Long:  "View the logs from services defined in an ignition-compose.yml file.",
		RunE: func(_ *cobra.Command, args []string) error {
			// Removed redundant operation line

			// Parse the compose file
			composeManifest, err := manifest.ParseComposeFile(filePath)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to parse compose file: %v", err))
				return err
			}

			// Get the engine client from the container
			client, err := container.Get("engineClient")
			if err != nil {
				ui.PrintError("Failed to get engine client")
				return fmt.Errorf("failed to get engine client: %w", err)
			}
			engineClient, ok := client.(*services.EngineClient)
			if !ok {
				ui.PrintError("Invalid engine client type")
				return errors.New("invalid engine client type")
			}

			// Check if engine is running
			if err := engineClient.Ping(context.Background()); err != nil {
				ui.PrintError(fmt.Sprintf("Engine is not running: %v", err))
				return fmt.Errorf("engine is not running: %w", err)
			}

			// Filter services by name if provided
			servicesToShow := make(map[string]manifest.ComposeService)
			if len(args) > 0 {
				for _, serviceName := range args {
					service, exists := composeManifest.Services[serviceName]
					if !exists {
						ui.PrintError(fmt.Sprintf("Service '%s' not found in compose file", serviceName))
						return fmt.Errorf("service '%s' not found in compose file", serviceName)
					}
					servicesToShow[serviceName] = service
				}
			} else {
				servicesToShow = composeManifest.Services
			}

			// If no services to show, exit early
			if len(servicesToShow) == 0 {
				ui.PrintInfo("Status", "No services found to show logs for")
				return nil
			}

			// Parse the since duration
			var sinceDuration time.Duration
			if since != "" {
				sinceDuration, err = time.ParseDuration(since)
				if err != nil {
					ui.PrintError(fmt.Sprintf("Invalid duration: %v", err))
					return fmt.Errorf("invalid duration format: %w", err)
				}
			}

			// Show logs once
			spinnerModel := spinner.NewSpinnerModelWithMessage("Retrieving logs...")
			program := tea.NewProgram(spinnerModel)

			go func() {
				logs, err := retrieveLogs(context.Background(), servicesToShow, engineClient, sinceDuration, tail)
				if err != nil {
					program.Send(spinner.ErrorMsg{Err: err})
				} else {
					program.Send(spinner.DoneMsg{Result: logs})
				}
			}()

			model, err := program.Run()
			if err != nil {
				ui.PrintError(fmt.Sprintf("UI error: %v", err))
				return err
			}

			finalModel, ok := model.(spinner.Model)
			if !ok {
				return errors.New("unexpected model type returned from spinner")
			}
			if finalModel.HasError() {
				return finalModel.GetError()
			}

			result := finalModel.GetResult()
			logs, ok := result.(map[string][]string)
			if !ok {
				return errors.New("unexpected result type: expected map[string][]string")
			}

			// Display logs
			displayLogs(logs)

			// If not following, exit now
			if !follow {
				return nil
			}

			// Follow logs in real-time
			ui.PrintInfo("Status", "Following logs (press Ctrl+C to exit)...")
			lastSeen := time.Now()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Setup ticker for periodic log fetching
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			// Follow logs until interrupted
			for {
				select {
				case <-ticker.C:
					// Get only new logs since last check
					newLogs, err := retrieveLogs(ctx, servicesToShow, engineClient, time.Since(lastSeen), 0)
					if err != nil {
						ui.PrintError(fmt.Sprintf("Error retrieving logs: %v", err))
						return err
					}

					if hasNewLogs(newLogs) {
						displayLogs(newLogs)
						lastSeen = time.Now()
					}
				case <-ctx.Done():
					return nil
				}
			}
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&follow, "follow", "F", false, "Follow log output")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since timestamp (e.g., 30m for 30 minutes)")
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of lines to show from the end of the logs")

	return cmd
}

// retrieveLogs gets logs for the specified services.
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
			return nil, fmt.Errorf("failed to retrieve logs for service '%s': %w", name, err)
		}

		logs[name] = functionLogs
	}

	return logs, nil
}

// displayLogs formats and displays logs with service name prefixes.
func displayLogs(logs map[string][]string) {
	for serviceName, serviceLog := range logs {
		if len(serviceLog) == 0 {
			continue
		}

		for _, line := range serviceLog {
			// Format each log line with service name prefix
			ui.PrintServiceLog(serviceName, line)
		}
	}
}

// hasNewLogs checks if there are any new log lines to display.
func hasNewLogs(logs map[string][]string) bool {
	for _, logLines := range logs {
		if len(logLines) > 0 {
			return true
		}
	}
	return false
}
