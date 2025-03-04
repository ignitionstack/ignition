package cmd

import (
	"os"
	"path/filepath"

	globalConfig "github.com/ignitionstack/ignition/internal/config"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/pkg/engine/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Global flags
var (
	socketPath        string
	engineClient      *services.EngineClient
	defaultSocketPath string
)

var rootCmd = &cobra.Command{
	Use:   "ignition",
	Short: "Ignition Stack CLI",
	Long: `Ignition is a WebAssembly function management and orchestration platform.

It allows you to build, deploy, and run WebAssembly functions across various
languages (JavaScript, Rust, Go) with a consistent interface and developer experience.

Key capabilities:
* Build WebAssembly functions from source code
* Manage function deployments with a local registry
* Run functions on the Ignition engine
* Call functions with custom payloads
* Manage function deployments with compose files`,
	Example: `  # Initialize a new JavaScript function
  ignition init my-function --template javascript

  # Build a function
  ignition build

  # Run the engine
  ignition engine start

  # Run a function
  ignition run ./my-function.wasm

  # List all functions
  ignition function list

  # Use a custom config file
  ignition --config ~/.ignition/custom-config.yaml function list`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Skip for help commands
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}

		// Skip loading config for engine start command, as it handles config directly
		if cmd.CommandPath() == "ignition engine start" {
			return nil
		}

		// Setup engine client from config
		err := setupEngineClient()
		if err != nil {
			// Don't return error, as some commands don't need the engine
			// Just silently continue with default client
			engineClient = services.NewEngineClientWithDefaults()
		}

		// Check if any command in the hierarchy has a plain flag set to true
		plainFlag := false
		cmd.Flags().Visit(func(f *pflag.Flag) {
			if f.Name == "plain" && f.Value.String() == "true" {
				plainFlag = true
			}
		})

		if !plainFlag {
			ui.PrintLogo()
		}

		return nil
	},
}

// Container holds the dependency injection container.
var Container = di.NewContainer()

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Set default socket path from global config
	defaultSocketPath = globalConfig.DefaultSocket

	// Add global flags
	rootCmd.PersistentFlags().StringVarP(&globalConfig.ConfigPath, "config", "c", config.DefaultConfigPath, "Path to the configuration file")
	rootCmd.PersistentFlags().StringVarP(&socketPath, "socket", "s", defaultSocketPath, "Path to the engine socket (overrides config)")

	// Register services in the container
	functionService := services.NewFunctionService()
	Container.Register("functionService", functionService)

	// Engine client will be initialized in setupEngineClient()
	// We register a default one for now, it will be replaced in PersistentPreRunE
	Container.Register("engineClient", services.NewEngineClientWithDefaults())
}

// setupEngineClient creates an engine client using the config file or command line flags
func setupEngineClient() error {
	// If socket path is explicitly provided, use it directly
	if socketPath != "" {
		client, err := services.NewEngineClient(socketPath)
		if err != nil {
			return err
		}

		engineClient = client
		Container.Register("engineClient", client)
		return nil
	}

	// Try to load from config file
	cfg, err := loadEngineConfig()
	if err != nil {
		return err
	}

	// Create client using socket path from config
	client, err := services.NewEngineClient(cfg.Server.SocketPath)
	if err != nil {
		return err
	}

	engineClient = client
	Container.Register("engineClient", client)
	return nil
}

// loadEngineConfig loads the engine configuration from the specified path
func loadEngineConfig() (*config.Config, error) {
	// Expand tilde in config path if needed
	expandedPath := globalConfig.ConfigPath
	if len(globalConfig.ConfigPath) > 0 && globalConfig.ConfigPath[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expandedPath = filepath.Join(homeDir, globalConfig.ConfigPath[1:])
		}
	}

	// Try to load config
	cfg, err := config.LoadConfig(expandedPath)
	if err != nil {
		// Fall back to default config
		return config.DefaultConfig(), nil
	}

	return cfg, nil
}
