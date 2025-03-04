package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/engine/models"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/spf13/cobra"
)

func NewComposeUpCommand(container *di.Container) *cobra.Command {
	var filePath string
	var detach bool

	cmd := &cobra.Command{
		Use:           "up",
		Short:         "Create and start functions defined in a compose file",
		Long:          "Create and start functions defined in an ignition-compose.yml file.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			composeManifest, err := manifest.ParseComposeFile(filePath)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to parse compose file: %v", err))
				return err
			}

			// Get the engine client from the container
			client, err := container.Get("engineClient")
			if err != nil {
				ui.PrintError(fmt.Sprintf("Error getting engine client: %v", err))
				return err
			}
			engineClient, ok := client.(*services.EngineClient)
			if !ok {
				ui.PrintError("Invalid engine client type")
				return errors.New("invalid engine client type")
			}

			// Check if engine is running
			if err := engineClient.Status(context.Background()); err != nil {
				ui.PrintError(fmt.Sprintf("Failed to connect to engine: %v", err))
				return err
			}

			// Create a context that we can cancel
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create a channel to listen for OS signals
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			// Start a goroutine to handle signals
			done := make(chan bool, 1)
			go func() {
				// Wait for a signal
				<-sigs

				// Print a message
				ui.PrintInfo("Status", "Shutting down...")

				// Cancel the context to signal all operations to stop
				cancel()

				var functionsToUnload []models.FunctionReference

				for name, service := range composeManifest.Services {
					parts := strings.Split(service.Function, ":")
					functionRef := parts[0]
					nameParts := strings.Split(functionRef, "/")
					if len(nameParts) == 2 {
						namespace, funcName := nameParts[0], nameParts[1]
						functionsToUnload = append(functionsToUnload, models.FunctionReference{
							Namespace: namespace,
							Name:      funcName,
							Service:   name,
						})
					}
				}

				spinnerModel := spinner.NewSpinnerModelWithMessage("Unloading functions...")
				unloadProgram := tea.NewProgram(spinnerModel)

				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					err := engineClient.StopFunctions(ctx, functionsToUnload)
					if err != nil {
						if isConnectionError(err) {
							unloadProgram.Send(spinner.DoneMsg{Result: 0})
						} else {
							unloadProgram.Send(spinner.ErrorMsg{Err: err})
						}
					} else {
						unloadProgram.Send(spinner.DoneMsg{Result: len(functionsToUnload)})
					}
				}()

				unloadModel, err := unloadProgram.Run()
				if err != nil {
					ui.PrintError(fmt.Sprintf("UI error: %v", err))
				} else {
					finalUnloadModel, ok := unloadModel.(spinner.Model)
					if !ok {
						ui.PrintError("Unexpected model type returned from spinner")
					} else {
						if finalUnloadModel.HasError() {
							ui.PrintError(fmt.Sprintf("Failed to unload functions: %v", finalUnloadModel.GetError()))
						} else {
							result := finalUnloadModel.GetResult()
							unloadedCount, ok := result.(int)
							if !ok {
								ui.PrintError("Unexpected result type")
							} else {
								ui.PrintSuccess(fmt.Sprintf("Successfully unloaded %d functions", unloadedCount))
							}
						}
					}
				}

				// Signal that we're done
				done <- true
			}()

			// Create spinner model for function loading
			spinnerModel := spinner.NewSpinnerModelWithMessage("Loading functions...")
			program := tea.NewProgram(spinnerModel)

			// Load functions in a goroutine
			go func() {
				result, err := loadFunctions(ctx, composeManifest, engineClient)
				if err != nil {
					program.Send(spinner.ErrorMsg{Err: err})
				} else {
					program.Send(spinner.DoneMsg{Result: result})
				}
			}()

			// Run the program to show the spinner
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
			loadedCount, ok := result.(int)
			if !ok {
				return errors.New("unexpected result type: expected int")
			}

			ui.PrintSuccess(fmt.Sprintf("Successfully loaded %d functions", loadedCount))

			fmt.Println()

			// If detach is true, return now
			if !detach {
				ui.PrintInfo("Status", "Waiting for Ctrl+C to shut down...")
				fmt.Println()
				fmt.Println(ui.DimStyle.Render("Functions are running. Press Ctrl+C to stop..."))

				// Wait for shutdown to complete
				<-done
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run functions in the background")

	return cmd
}

// Check if an error is a connection-related error.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "no such file or directory") ||
		strings.Contains(errMsg, "cannot connect to the engine") ||
		strings.Contains(errMsg, "engine is not running")
}

func loadFunctions(ctx context.Context, composeManifest *manifest.ComposeManifest, engineClient *services.EngineClient) (int, error) {
	loadedCount := 0
	var loadErrs []string

	for name, service := range composeManifest.Services {
		// Parse function reference (namespace/name:tag)
		parts := strings.Split(service.Function, ":")
		if len(parts) != 2 {
			loadErrs = append(loadErrs, fmt.Sprintf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name))
			continue
		}

		functionRef, tag := parts[0], parts[1]

		// Parse namespace and name
		nameParts := strings.Split(functionRef, "/")
		if len(nameParts) != 2 {
			loadErrs = append(loadErrs, fmt.Sprintf("invalid function reference '%s' for service '%s', expected format namespace/name:tag", service.Function, name))
			continue
		}

		namespace, funcName := nameParts[0], nameParts[1]

		// Create configuration by merging both Config and Environment fields
		config := make(map[string]string)

		// First copy from legacy Config field if present
		for k, v := range service.Config {
			config[k] = v
		}

		// Then copy from Environment field, which takes precedence
		for k, v := range service.Environment {
			config[k] = v
		}

		// Load the function
		err := engineClient.LoadFunction(ctx, namespace, funcName, tag, config)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to load function '%s' for service '%s': %v", service.Function, name, err)

			// Provide more helpful error messages for common issues
			if strings.Contains(err.Error(), "function not found") {
				errorMsg = fmt.Sprintf("Function '%s' not found for service '%s'. Run 'ignition function build' to create it first.",
					service.Function, name)
			} else if strings.Contains(err.Error(), "no such file or directory") {
				errorMsg = fmt.Sprintf("Unable to load function '%s' for service '%s'. The function file does not exist.",
					service.Function, name)
			}

			loadErrs = append(loadErrs, errorMsg)
			continue
		}

		loadedCount++
	}

	// Check for errors
	if len(loadErrs) > 0 {
		return loadedCount, fmt.Errorf("failed to load some functions:\n%s", strings.Join(loadErrs, "\n"))
	}

	return loadedCount, nil
}