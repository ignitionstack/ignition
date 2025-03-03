package cmd

import (
	"github.com/ignitionstack/ignition/cmd/function"
	"github.com/spf13/cobra"
)

var LocalRegistryPath string

var functionCmd = &cobra.Command{
	Use:   "function",
	Short: "Manage WebAssembly functions",
	Long: `Commands for working with WebAssembly functions in the Ignition platform.

Functions are the core unit of deployment in Ignition. This command group provides
tools for managing functions throughout their lifecycle:
* List available functions in the registry
* Inspect function metadata
* Manage function tags and versions

Functions have a namespace/name format and can be tagged with version identifiers
for easier management.`,
	Example: `  # List all functions in the registry
  ignition function list

  # View details of a specific function
  ignition function list my-namespace/my-function`,
	Aliases: []string{"fn"},
}

func init() {
	rootCmd.AddCommand(function.NewFunctionInitCommand())
	rootCmd.AddCommand(function.NewFunctionBuildCommand())
	rootCmd.AddCommand(function.NewFunctionCallCommand())
	rootCmd.AddCommand(function.NewFunctionRunCommand())
	rootCmd.AddCommand(function.NewFunctionStopCommand())
	rootCmd.AddCommand(function.NewFunctionTagCommand())

	functionCmd.AddCommand(function.NewFunctionListCommand())
	rootCmd.AddCommand(functionCmd)
}
