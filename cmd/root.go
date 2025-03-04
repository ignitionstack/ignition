package cmd

import (
	"os"

	globalConfig "github.com/ignitionstack/ignition/internal/config"
	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
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
  ignition function list`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Skip for help commands
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}

		// Skip for engine start command, as it handles configuration directly
		if cmd.CommandPath() == "ignition engine start" {
			return nil
		}

		// Setup engine client with socket path
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
	rootCmd.PersistentFlags().StringVarP(&socketPath, "socket", "s", defaultSocketPath, "Path to the engine socket")

	// Register services in the container
	functionService := services.NewFunctionService()
	Container.Register("functionService", functionService)

	// Engine client will be initialized in setupEngineClient()
	// We register a default one for now, it will be replaced in PersistentPreRunE
	Container.Register("engineClient", services.NewEngineClientWithDefaults())
}

// setupEngineClient creates an engine client using the socket path
func setupEngineClient() error {
	// Create client using the socket path
	client, err := services.NewEngineClient(socketPath)
	if err != nil {
		return err
	}

	engineClient = client
	Container.Register("engineClient", client)
	return nil
}
