package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// NewEngineStartCommand creates a command to start the engine.
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
		Long: `Start the Ignition WebAssembly runtime engine server.

The engine server provides:
* A runtime environment for WebAssembly modules
* Function lifecycle management
* Local registry integration
* HTTP and socket-based APIs
* Circuit breaker patterns for resilience

The engine must be running for other Ignition commands like function calls to work.
The engine can be configured with various flags to customize its behavior.`,
		Example: `  # Start the engine with default settings
  ignition engine start

  # Start with a custom socket path and HTTP port
  ignition engine start --socket /tmp/my-socket.sock --http :9090

  # Start with detailed logging
  ignition engine start --log-level debug --log-file /var/log/ignition.log`,
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

			// Print startup message
			fmt.Println("Starting Ignition Engine...")
			fmt.Println("Press Ctrl+C to stop")

			// Create app configuration for fx
			appConfig := di.NewAppConfig(
				config.socketPath,
				config.httpAddr,
				config.registryDir,
			)

			// Setup the fx app with our module
			app := fx.New(
				// Provide app configuration
				fx.Supply(appConfig),

				// Include all our dependency providers
				di.Module,

				// Register the engine start as an fx invocation
				fx.Invoke(func(engine *engine.Engine) {
					// The engine's Start method will block, which is what we want
					if err := engine.Start(); err != nil {
						// Log the error - we can't return it here because fx.Invoke doesn't
						// propagate errors up to RunE
						fmt.Fprintf(os.Stderr, "Engine server failed: %v\n", err)
						os.Exit(1)
					}
				}),

				// Configure fx options
				fx.StartTimeout(30*time.Second),
				fx.StopTimeout(30*time.Second),
			)

			// Start the application and wait for it to finish
			if err := app.Start(context.Background()); err != nil {
				return fmt.Errorf("failed to start engine: %w", err)
			}

			// This allows for graceful shutdown on SIGINT/SIGTERM
			<-app.Done()

			// Handle shutdown
			if err := app.Stop(context.Background()); err != nil {
				return fmt.Errorf("error during shutdown: %w", err)
			}

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

// ensureDirectoryExists creates a directory if it doesn't exist.
func ensureDirectoryExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}
