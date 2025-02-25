package cmd

import (
	"fmt"
	"os"

	"github.com/ignitionstack/ignition/internal/di"
	"github.com/ignitionstack/ignition/internal/services"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ignition",
	Short: "Ignition Stack CLI",
	Long:  ``,
}

// Container holds the dependency injection container
var Container = di.NewContainer()

func Execute() {
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
