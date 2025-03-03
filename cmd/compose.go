package cmd

import (
	"github.com/ignitionstack/ignition/cmd/compose"
	"github.com/spf13/cobra"
)

// ComposeCmd represents the compose command.
var ComposeCmd = &cobra.Command{
	Use:   "compose",
	Short: "Manage multiple functions with compose files",
	Long: `Compose allows you to define and run multi-function applications.
A compose file lets you configure multiple functions, their dependencies,
and their runtime configurations in a single YAML file.`,
}

func init() {
	// Add compose subcommands
	ComposeCmd.AddCommand(compose.NewComposeUpCommand(Container))
	ComposeCmd.AddCommand(compose.NewComposeDownCommand(Container))
	ComposeCmd.AddCommand(compose.NewComposeInitCommand(Container))
	ComposeCmd.AddCommand(compose.NewComposeLogsCommand(Container))

	rootCmd.AddCommand(ComposeCmd)
}
