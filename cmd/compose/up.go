package compose

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
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
		RunE: func(c *cobra.Command, args []string) error {
			ui.PrintInfo("Operation", "Starting compose services")

			composeManifest, err := manifest.ParseComposeFile(filePath)
			if err != nil {
				ui.PrintError(fmt.Sprintf("Failed to parse compose file: %v", err))
				return err
			}

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

			if err := engineClient.Ping(context.Background()); err != nil {
				ui.PrintError("Engine is not running")
				return fmt.Errorf("engine is not running. Start it with 'ignition engine start': %w", err)
			}

			ctx := context.Background()

			spinnerModel := spinner.NewSpinnerModelWithMessage("Loading functions from compose file...")
			program := tea.NewProgram(spinnerModel)

			go func() {
				result, err := loadFunctions(ctx, composeManifest, engineClient)
				if err != nil {
					program.Send(spinner.ErrorMsg{Err: err})
				} else {
					program.Send(spinner.DoneMsg{Result: result})
				}
			}()

			model, err := program.Run()
			if err != nil {
				ui.PrintError(fmt.Sprintf("UI error: %v", err))
				return err
			}

			finalModel := model.(spinner.SpinnerModel)
			if finalModel.HasError() {
				return finalModel.GetError()
			}

			loadedCount := finalModel.GetResult().(int)

			ui.PrintSuccess(fmt.Sprintf("Successfully started %d functions from compose file", loadedCount))

			ui.PrintInfo("Status", "Running functions")
			for name, service := range composeManifest.Services {
				ui.PrintMetadata(name, service.Function)
			}

			if !detach {
				ui.PrintInfo("Status", "Functions are running. Press Ctrl+C to stop...")

				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

				engineHealthChan := make(chan struct{})
				engineCheckCtx, engineCheckCancel := context.WithCancel(context.Background())
				defer engineCheckCancel()

				go func() {
					ticker := time.NewTicker(10 * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-ticker.C:
							pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
							err := engineClient.Ping(pingCtx)
							cancel()

							if err != nil {
								ui.PrintWarning("Engine is no longer running. Stopping compose services.")
								close(engineHealthChan)
								return
							}
						case <-engineCheckCtx.Done():
							return
						}
					}
				}()

				select {
				case <-sigChan:
					// User pressed Ctrl+C
				case <-engineHealthChan:
					// Engine is no longer running
				}

				ui.PrintInfo("Operation", "Shutting down and unloading functions...")

				var functionsToUnload []struct {
					namespace string
					name      string
					service   string
				}

				for name, service := range composeManifest.Services {
					parts := strings.Split(service.Function, ":")
					functionRef := parts[0]
					nameParts := strings.Split(functionRef, "/")
					if len(nameParts) == 2 {
						namespace, funcName := nameParts[0], nameParts[1]
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
				}

				spinnerModel := spinner.NewSpinnerModelWithMessage("Unloading functions...")
				unloadProgram := tea.NewProgram(spinnerModel)

				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					err := unloadComposeServices(ctx, functionsToUnload, engineClient)
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
					ui.PrintError(fmt.Sprintf("Error unloading functions: %v", err))
				} else {
					finalUnloadModel := unloadModel.(spinner.SpinnerModel)
					if finalUnloadModel.HasError() {
						ui.PrintError(fmt.Sprintf("Error unloading functions: %v", finalUnloadModel.GetError()))
					} else {
						unloadedCount := finalUnloadModel.GetResult().(int)
						ui.PrintSuccess(fmt.Sprintf("Successfully unloaded %d functions", unloadedCount))
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Specify an alternate compose file (default: ignition-compose.yml)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run in detached mode")

	return cmd
}

func loadFunctions(ctx context.Context, composeManifest *manifest.ComposeManifest, engineClient *services.EngineClient) (int, error) {
	var loadErrs []string
	var loadErrsMu sync.Mutex
	var wg sync.WaitGroup

	for name, service := range composeManifest.Services {
		wg.Add(1)
		go func(name string, service manifest.ComposeService) {
			defer wg.Done()

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

			err := engineClient.LoadFunction(ctx, namespace, funcName, tag)
			if err != nil {
				errorMsg := fmt.Sprintf("Failed to load function '%s' for service '%s'", service.Function, name)

				if strings.Contains(err.Error(), "function not found") {
					errorMsg = fmt.Sprintf("Function '%s' not found for service '%s'. Run 'ignition function build' to create it first.",
						service.Function, name)
				} else if strings.Contains(err.Error(), "engine is not running") {
					errorMsg = "Engine is not running. Start it with 'ignition engine start' before running compose up."
				} else {
					errorMsg = fmt.Sprintf("%s: %v", errorMsg, err)
				}

				loadErrsMu.Lock()
				loadErrs = append(loadErrs, errorMsg)
				loadErrsMu.Unlock()
				return
			}
		}(name, service)
	}

	wg.Wait()

	if len(loadErrs) > 0 {
		errorMessage := "Failed to start services:\n"
		for _, err := range loadErrs {
			errorMessage += fmt.Sprintf("• %s\n", err)
		}
		return 0, fmt.Errorf("%s", errorMessage)
	}

	return len(composeManifest.Services), nil
}

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

func unloadComposeServices(ctx context.Context, functions []struct {
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

	wg.Wait()

	if len(unloadErrs) > 0 {
		return fmt.Errorf("failed to unload some functions:\n%s", strings.Join(unloadErrs, "\n"))
	}

	return nil
}
