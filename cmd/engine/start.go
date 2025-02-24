package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/spf13/cobra"
)

// NewEngineStartCommand creates a command to start the engine
func NewEngineStartCommand() *cobra.Command {
	// Configuration options
	var config struct {
		socketPath  string
		httpAddr    string
		registryDir string
		logFile     string
		logLevel    string
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the engine server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setup registry directory
			if config.registryDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get user home directory: %w", err)
				}
				config.registryDir = filepath.Join(homeDir, ".ignition")
			}

			// Ensure registry directory exists
			if err := ensureDirectoryExists(config.registryDir); err != nil {
				return err
			}

			// Create a simple logger
			logger := engine.NewStdLogger(os.Stdout)
			
			// Create and configure the engine
			engineInstance, err := engine.NewEngineWithLogger(
				config.socketPath,
				config.httpAddr,
				config.registryDir,
				logger,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize engine: %w", err)
			}

			// Print startup message
			fmt.Println("Starting Ignition engine...")
			
			// Start the engine
			if err := engineInstance.Start(); err != nil {
				return fmt.Errorf("engine server failed: %w", err)
			}

			// We should never reach here since Start() blocks
			return nil
		},
	}

	// Register command flags
	cmd.Flags().StringVarP(&config.socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringVarP(&config.httpAddr, "http", "H", ":8080", "HTTP server address")
	cmd.Flags().StringVarP(&config.registryDir, "directory", "d", "", "Registry directory ($HOME/.ignition if empty)")
	cmd.Flags().StringVarP(&config.logFile, "log-file", "l", "", "Log file path (logs to stdout if not specified)")
	cmd.Flags().StringVarP(&config.logLevel, "log-level", "L", "info", "Log level (error, info, debug)")

	return cmd
}

// ensureDirectoryExists creates a directory if it doesn't exist
func ensureDirectoryExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}