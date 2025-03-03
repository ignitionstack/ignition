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
	"github.com/ignitionstack/ignition/pkg/types"
	"github.com/spf13/cobra"
)

func NewFunctionRunCommand() *cobra.Command {
	var runSocketPath string
	var runConfigFlag []string
	cmd := &cobra.Command{
		Use:           "run [namespace/name:identifier]",
		Short:         "Load and optionally run a WASM file from the registry on the engine",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, name, identifier, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			spinnerModel := spinner.NewSpinnerModelWithMessage("Loading...")
			p := tea.NewProgram(spinnerModel)

			go func() {
				loadStart := time.Now()

				// Parse config flag values into a map
				config := make(map[string]string)
				for _, configItem := range runConfigFlag {
					parts := splitKeyValue(configItem)
					if len(parts) == 2 {
						config[parts[0]] = parts[1]
					}
				}

				req := map[string]interface{}{
					"namespace":  namespace,
					"name":       name,
					"digest":     identifier,
					"force_load": true, // Always force load when running with function run command
				}

				// Only add config if there are values
				if len(config) > 0 {
					req["config"] = config
				}

				reqBody, err := json.Marshal(req)
				if err != nil {
					p.Send(fmt.Errorf("failed to encode request: %w", err))
					return
				}

				client := http.Client{
					Transport: &http.Transport{
						DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", runSocketPath)
						},
					},
				}

				resp, err := client.Post("http://unix/load", "application/json", bytes.NewBuffer(reqBody))
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

				loadTime := time.Since(loadStart)
				p.Send(spinner.ResultMsg{Result: types.LoadResult{
					Namespace: namespace,
					Name:      name,
					Digest:    identifier,
					LoadTime:  loadTime,
				}})
			}()

			m, err := p.Run()
			if err != nil {
				return err
			}

			finalModel := m.(spinner.SpinnerModel)
			if finalModel.HasError() {
				err := finalModel.GetError()
				return err
			}

			ui.PrintSuccess("Function loaded successfully")
			return nil
		},
	}

	cmd.Flags().StringVarP(&runSocketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringArrayVarP(&runConfigFlag, "config", "c", []string{}, "Configuration values to pass to the function (format: key=value)")
	return cmd
}

// Helper function to split a key=value string
func splitKeyValue(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
