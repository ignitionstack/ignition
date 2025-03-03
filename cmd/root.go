package cmd

import (
	"fmt"
	"os"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
}

// Container holds the dependency injection container.
var Container = di.NewContainer()

func Execute() {
	// Add logo display to root command with pre-run hook
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Skip logo for help commands and plain output
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return
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
	}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Register services in the container

	// Register the function service
	functionService := services.NewFunctionService()
	Container.Register("functionService", functionService)

	// Register the engine client with safe creation
	engineClient, err := services.NewEngineClient("/tmp/ignition-engine.sock")
	if err != nil {
		fmt.Printf("Warning: Failed to create engine client: %v\n", err)
		engineClient = services.NewEngineClientWithDefaults()
	}
	Container.Register("engineClient", engineClient)
}
