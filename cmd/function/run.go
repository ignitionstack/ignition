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

func NewFunctionRunCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "run [namespace/name:identifier]",
		Short: "Load and optionally run a WASM file from the registry on the engine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, name, identifier, err := parseNamespaceAndName(args[0])
			if err != nil {
				return fmt.Errorf("invalid function name format: %w", err)
			}

			p := tea.NewProgram(spinner.NewSpinnerModelWithMessage("Loading..."))

			go func() {
				loadStart := time.Now()

				// Send a request to the engine to load the function
				req := map[string]string{
					"namespace": namespace,
					"name":      name,
					"digest":    identifier,
					"tag":       identifier,
				}

				reqBody, err := json.Marshal(req)
				if err != nil {
					p.Send(fmt.Errorf("failed to encode request: %w", err))
					return
				}

				client := http.Client{
					Transport: &http.Transport{
						DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", socketPath)
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
					var errorMsg string
					if err := json.NewDecoder(resp.Body).Decode(&errorMsg); err != nil {
						errorMsg = "unknown error"
					}
					p.Send(fmt.Errorf("engine returned error: %s", errorMsg))
					return
				}

				loadTime := time.Since(loadStart)
				p.Send(spinner.ResultMsg{Result: engine.LoadResult{
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
				return finalModel.GetError()
			}

			ui.PrintSuccess("Function loaded successfully")
			return nil
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")

	return cmd
}
