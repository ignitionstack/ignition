package engine

import (
	"fmt"
	"os"

	globalConfig "github.com/ignitionstack/ignition/internal/config"
	"github.com/ignitionstack/ignition/pkg/engine"
	"github.com/ignitionstack/ignition/pkg/engine/config"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/spf13/cobra"
)

// NewEngineStartCommand creates a command to start the engine.
func NewEngineStartCommand() *cobra.Command {
	// Configuration options
	var cmdConfig struct {
		socketPath   string
		httpAddr     string
		registryDir  string
		logFile      string
		logLevel     string
		showConfig   bool
		defaultsOnly bool
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
The engine can be configured with various flags, environment variables, or a YAML config file.`,
		Example: `  # Start the engine with default settings
  ignition engine start

  # Start with a custom socket path and HTTP port
  ignition engine start --socket-path /tmp/my-socket.sock --http :9090

  # Start with a custom config file
  ignition engine start --config ~/.ignition/custom-config.yaml

  # Start with detailed logging
  ignition engine start --log-level debug --log-file /var/log/ignition.log`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// If the user just wants to see the config, print it and exit
			if cmdConfig.showConfig {
				cfg, err := loadConfig(globalConfig.ConfigPath, cmdConfig.defaultsOnly)
				if err != nil {
					return fmt.Errorf("failed to load configuration: %w", err)
				}

				fmt.Printf("Engine configuration:\n")
				fmt.Printf("  Server:\n")
				fmt.Printf("    SocketPath: %s\n", cfg.Server.SocketPath)
				fmt.Printf("    HTTPAddr: %s\n", cfg.Server.HTTPAddr)
				fmt.Printf("    RegistryDir: %s\n", cfg.Server.RegistryDir)
				fmt.Printf("  Engine:\n")
				fmt.Printf("    DefaultTimeout: %s\n", cfg.Engine.DefaultTimeout)
				fmt.Printf("    LogStoreCapacity: %d\n", cfg.Engine.LogStoreCapacity)
				fmt.Printf("    CircuitBreaker:\n")
				fmt.Printf("      FailureThreshold: %d\n", cfg.Engine.CircuitBreaker.FailureThreshold)
				fmt.Printf("      ResetTimeout: %s\n", cfg.Engine.CircuitBreaker.ResetTimeout)
				fmt.Printf("    PluginManager:\n")
				fmt.Printf("      TTL: %s\n", cfg.Engine.PluginManager.TTL)
				fmt.Printf("      CleanupInterval: %s\n", cfg.Engine.PluginManager.CleanupInterval)

				return nil
			}

			// Load configuration from file and environment variables
			cfg, err := loadConfig(globalConfig.ConfigPath, cmdConfig.defaultsOnly)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Override config with command line flags if provided
			if cmdConfig.socketPath != "" {
				cfg.Server.SocketPath = cmdConfig.socketPath
			}
			if cmdConfig.httpAddr != "" {
				cfg.Server.HTTPAddr = cmdConfig.httpAddr
			}
			if cmdConfig.registryDir != "" {
				cfg.Server.RegistryDir = cmdConfig.registryDir
			}

			// Ensure registry directory exists
			if err := ensureDirectoryExists(cfg.Server.RegistryDir); err != nil {
				return err
			}

			// Set up logger
			var logger logging.Logger
			if cmdConfig.logFile != "" {
				logFile, err := os.OpenFile(cmdConfig.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					return fmt.Errorf("failed to open log file: %w", err)
				}
				logger = logging.NewStdLogger(logFile)
			} else {
				logger = logging.NewStdLogger(os.Stdout)
			}

			// Print startup message
			fmt.Println("Starting Ignition Engine...")
			fmt.Println("Press Ctrl+C to stop")

			// Create the engine with our configuration
			eng, err := engine.NewEngineWithConfig(cfg, logger)
			if err != nil {
				return fmt.Errorf("failed to create engine: %w", err)
			}

			// Start the engine
			if err := eng.Start(); err != nil {
				return fmt.Errorf("engine server failed: %w", err)
			}

			return nil
		},
	}

	// Get default socket path from global config
	defaultSocketPath := globalConfig.DefaultSocket

	// Register command flags
	// Use a different flag name and shorthand for socket to avoid conflicts with root command
	cmd.Flags().StringVarP(&cmdConfig.socketPath, "socket-path", "S", defaultSocketPath, "Path to the Unix socket (overrides config)")
	cmd.Flags().StringVarP(&cmdConfig.httpAddr, "http", "H", "", "HTTP server address (overrides config)")
	cmd.Flags().StringVarP(&cmdConfig.registryDir, "directory", "d", "", "Registry directory (overrides config)")
	cmd.Flags().StringVarP(&cmdConfig.logFile, "log-file", "l", "", "Log file path (logs to stdout if not specified)")
	cmd.Flags().StringVarP(&cmdConfig.logLevel, "log-level", "L", "info", "Log level (error, info, debug)")
	// Don't define config flag as it's already defined in root command
	cmd.Flags().BoolVarP(&cmdConfig.showConfig, "show-config", "C", false, "Show the configuration and exit")
	cmd.Flags().BoolVar(&cmdConfig.defaultsOnly, "defaults-only", false, "Use only default configuration, ignore config file and env vars")

	return cmd
}

// loadConfig loads the configuration from the specified path and environment variables.
func loadConfig(configPath string, defaultsOnly bool) (*config.Config, error) {
	if defaultsOnly {
		return config.DefaultConfig(), nil
	}
	return config.LoadConfig(configPath)
}

// ensureDirectoryExists creates a directory if it doesn't exist.
func ensureDirectoryExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}
