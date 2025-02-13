package cmd

import (
	"github.com/ignitionstack/ignition/cmd/function"
	"github.com/spf13/cobra"
)

var LocalRegistryPath string

var functionCmd = &cobra.Command{
	Use:   "function",
	Short: "function related commands",
}

func init() {
	rootCmd.AddCommand(function.NewFunctionInitCommand())
	rootCmd.AddCommand(function.NewFunctionBuildCommand())
	rootCmd.AddCommand(function.NewFunctionCallCommand())
	rootCmd.AddCommand(function.NewFunctionRunCommand())
	rootCmd.AddCommand(function.NewFunctionTagCommand())

	functionCmd.AddCommand(function.NewFunctionListCommand())
	rootCmd.AddCommand(functionCmd)
}
