package cmd

import (
	"github.com/ignitionstack/ignition/cmd/engine"
	"github.com/spf13/cobra"
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Manage the Ignition engine",
	Long: `Commands for controlling the Ignition WebAssembly runtime engine.

The engine is responsible for running and managing WebAssembly modules. It provides:
* A runtime environment for WebAssembly functions
* Socket-based API for function management
* Registry integration for function storage and versioning
* Lifecycle management for function instances

The engine must be running to deploy or call functions.`,
	Example: `  # Start the engine with default settings
  ignition engine start

  # Start the engine with a custom socket path
  ignition engine start --socket /tmp/custom-socket.sock

  # Start the engine with a custom registry directory
  ignition engine start --directory /path/to/registry`,
}

func init() {
	engineCmd.AddCommand(engine.NewEngineStartCommand())

	rootCmd.AddCommand(engineCmd)
}
