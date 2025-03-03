package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/spf13/cobra"
)

func NewFunctionStopCommand() *cobra.Command {
	var stopSocketPath string
	cmd := &cobra.Command{
		Use:   "stop [namespace/name]",
		Short: "Stop a function and prevent automatic reloading",
		Long: `Stop a function completely and prevent it from being automatically reloaded.

This command fully unloads the function and marks it as stopped, which prevents
it from automatically reloading when called. The function will only be loaded again
if explicitly loaded with 'ignition function run'.

Stopped functions will still appear in 'ignition ps' but with a "stopped" status.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			namespace, name, _, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			spinnerModel := spinner.NewSpinnerModelWithMessage("Stopping function...")
			p := tea.NewProgram(spinnerModel)

			go func() {
				stopStart := time.Now()

				req := map[string]interface{}{
					"namespace": namespace,
					"name":      name,
				}

				reqBody, err := json.Marshal(req)
				if err != nil {
					p.Send(fmt.Errorf("failed to encode request: %w", err))
					return
				}

				client := http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", stopSocketPath)
						},
					},
				}

				resp, err := client.Post("http://unix/stop", "application/json", bytes.NewBuffer(reqBody))
				if err != nil {
					p.Send(fmt.Errorf("failed to send request to engine: %w", err))
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					var errResp engine.RequestError
					if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
						p.Send(fmt.Errorf("failed to decode error response: %w", err))
						return
					}
					p.Send(errResp)
					return
				}

				stopTime := time.Since(stopStart)
				p.Send(spinner.ResultMsg{Result: map[string]interface{}{
					"namespace": namespace,
					"name":      name,
					"stopTime":  stopTime,
				}})
			}()

			m, err := p.Run()
			if err != nil {
				return err
			}

			finalModel, ok := m.(spinner.Model)
			if !ok {
				return fmt.Errorf("unexpected model type returned from spinner")
			}
			if finalModel.HasError() {
				err := finalModel.GetError()
				return err
			}

			ui.PrintSuccess(fmt.Sprintf("Function %s/%s stopped successfully", namespace, name))
			return nil
		},
	}

	cmd.Flags().StringVarP(&stopSocketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	return cmd
}
