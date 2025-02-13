package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/spf13/cobra"
)

func NewEngineStartCommand() *cobra.Command {
	var socketPath string
	var httpAddr string
	var registryDir string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the engine server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if registryDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get user home directory: %w", err)
				}

				registryDir = filepath.Join(homeDir, ".ignition")
			}

			engine, err := engine.NewEngine(socketPath, httpAddr, registryDir)
			if err != nil {
				return fmt.Errorf("failed to initialize engine: %w", err)
			}

			return engine.Start()
		},
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringVarP(&httpAddr, "http", "H", ":8080", "HTTP server address")
	cmd.Flags().StringVarP(&registryDir, "directory", "d", "", "Registry directory ($HOME/.ignition if empty")

	return cmd
}
