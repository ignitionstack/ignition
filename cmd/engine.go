package cmd

import (
	"github.com/ignitionstack/ignition/cmd/engine"
	"github.com/spf13/cobra"
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "engine related commands",
}

func init() {
	engineCmd.AddCommand(engine.NewEngineStartCommand())

	rootCmd.AddCommand(engineCmd)
}
