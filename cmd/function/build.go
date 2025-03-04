package function

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/engine/api"
	"github.com/ignitionstack/ignition/pkg/engine/client"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var socketPath string

type TagInfo struct {
	Namespace string
	Name      string
	Tag       string
}

func NewFunctionBuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build [path]",
		Short: "Build a WebAssembly function",
		Long: `Build a WebAssembly function from source code.

This command compiles a function defined in an ignition.yml manifest file into a 
WebAssembly module, and registers it in the local registry. The build process:

1. Reads the function configuration from ignition.yml
2. Identifies the appropriate builder based on the function language
3. Builds the source code into a WebAssembly module
4. Stores the built function in the registry with specified tags
5. Makes the function available for deployment

The build command requires a running engine to perform the compilation process.`,
		Example: `  # Build function in the current directory
  ignition build

  # Build function in a specific directory
  ignition build ./path/to/function

  # Build and tag the function
  ignition build -t namespace/name:tag

  # Build with multiple tags
  ignition build -t namespace/name:latest -t namespace/name:v1.0.0`,
		Args:          cobra.MaximumNArgs(1),
		RunE:          buildFunction,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Use the default socket path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	defaultSocketPath := filepath.Join(homeDir, ".ignition", "engine.sock")

	cmd.Flags().StringVarP(&socketPath, "socket", "s", defaultSocketPath, "Path to the Unix socket")
	cmd.Flags().StringArrayP("tag", "t", []string{}, "Tags for the function (can be specified multiple times)")

	return cmd
}

func buildFunction(cmd *cobra.Command, args []string) error {
	// Validate and prepare the build directory
	absPath, err := validateAndPrepareBuildDir(args)
	if err != nil {
		return err
	}

	// Load and parse the function manifest
	functionConfig, err := loadFunctionManifest(absPath)
	if err != nil {
		return err
	}

	// Parse tags from command line flags
	tags, err := parseTags(cmd, absPath)
	if err != nil {
		return err
	}

	// Create engine client
	engineClient, err := client.New(client.Options{
		SocketPath: socketPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create engine client: %w", err)
	}

	// Start the build spinner
	spinnerModel := spinner.NewSpinnerModelWithMessage("Building...")
	program := tea.NewProgram(spinnerModel)

	// Run the build in a goroutine to allow the spinner to update
	go runBuild(program, absPath, tags, functionConfig, engineClient)

	// Run the UI program and wait for completion
	model, err := program.Run()
	if err != nil {
		return err
	}

	// Check for build errors
	finalModel, ok := model.(spinner.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", model)
	}
	if finalModel.HasError() {
		return finalModel.GetError()
	}

	// Display build results
	resultValue := finalModel.GetResult()
	result, ok := resultValue.(types.BuildResult)
	if !ok {
		return fmt.Errorf("unexpected result type: %T", resultValue)
	}
	displayBuildResults(result, tags)

	return nil
}

func validateAndPrepareBuildDir(args []string) (string, error) {
	// Use current directory if no path provided
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("path %s does not exist", absPath))
		return "", fmt.Errorf("path does not exist: %w", err)
	}

	// Check for ignition.yml file
	manifestPath := filepath.Join(absPath, "ignition.yml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("no ignition.yml found in %s", absPath))
		return "", fmt.Errorf("ignition.yml not found: %w", err)
	}

	return absPath, nil
}

func loadFunctionManifest(path string) (manifest.FunctionManifest, error) {
	var config manifest.FunctionManifest

	// Read ignition.yml file
	manifestPath := filepath.Join(path, "ignition.yml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return config, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(manifestData, &config); err != nil {
		return config, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if config.FunctionSettings.Name == "" {
		return config, errors.New("function name is required in ignition.yml")
	}

	if config.FunctionSettings.Language == "" {
		return config, errors.New("function language is required in ignition.yml")
	}

	return config, nil
}

func parseTags(cmd *cobra.Command, absPath string) ([]TagInfo, error) {
	// Get tag flags
	tagFlags, err := cmd.Flags().GetStringArray("tag")
	if err != nil {
		return nil, fmt.Errorf("failed to get tag flags: %w", err)
	}

	// If no tags provided, try to read from the manifest
	if len(tagFlags) == 0 {
		// Read ignition.yml file
		manifestPath := filepath.Join(absPath, "ignition.yml")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read manifest: %w", err)
		}

		// Parse YAML
		var config manifest.FunctionManifest
		if err := yaml.Unmarshal(manifestData, &config); err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}

		// Use the function name from the manifest with default namespace
		if config.FunctionSettings.Name != "" {
			namespace := "default" // Always use default namespace
			tagFlags = append(tagFlags, fmt.Sprintf("%s/%s:latest", namespace, config.FunctionSettings.Name))
		}
	}

	// Create tag info objects from tag strings
	var tags []TagInfo
	for _, tag := range tagFlags {
		namespace, name, tagValue, err := parseTag(tag)
		if err != nil {
			return nil, err
		}
		tags = append(tags, TagInfo{
			Namespace: namespace,
			Name:      name,
			Tag:       tagValue,
		})
	}

	return tags, nil
}

// parseTag parses a tag in the format namespace/name:tag
func parseTag(tag string) (namespace, name, tagValue string, err error) {
	// Split namespace and name from tag
	parts := strings.Split(tag, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid tag format: %s (expected namespace/name:tag)", tag)
	}

	namespace = parts[0]
	nameTag := parts[1]

	// Split name from tag
	parts = strings.Split(nameTag, ":")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid tag format: %s (expected namespace/name:tag)", tag)
	}

	name = parts[0]
	tagValue = parts[1]

	// Check that all parts are non-empty
	if namespace == "" || name == "" || tagValue == "" {
		return "", "", "", fmt.Errorf("invalid tag format: %s (all parts must be non-empty)", tag)
	}

	return namespace, name, tagValue, nil
}

// runBuild executes the build process and updates the spinner with progress.
func runBuild(program *tea.Program, absPath string, tags []TagInfo,
	functionConfig manifest.FunctionManifest, client api.Client) {
	buildStart := time.Now()
	var finalResult *types.BuildResult

	// Send build requests for each tag
	for _, tagInfo := range tags {
		// Create build request
		req := api.BuildRequest{
			BaseRequest: api.BaseRequest{
				Namespace: tagInfo.Namespace,
				Name:      tagInfo.Name,
			},
			Path:     absPath,
			Tag:      tagInfo.Tag,
			Manifest: functionConfig,
		}

		// Send build request
		result, err := client.BuildFunction(context.Background(), req)
		if err != nil {
			program.Send(err)
			return
		}

		// Convert models.BuildResult to types.BuildResult
		typesResult := &types.BuildResult{
			Name:      result.BuildResult.Name,
			Namespace: result.BuildResult.Namespace,
			Digest:    result.BuildResult.Digest,
			BuildTime: result.BuildResult.BuildTime,
			Tag:       result.BuildResult.Tag,
			Reused:    result.BuildResult.Reused,
		}
		finalResult = typesResult
	}

	if finalResult != nil {
		finalResult.BuildTime = time.Since(buildStart).String()
		program.Send(spinner.ResultMsg{Result: *finalResult})
	}
}

// displayBuildResults shows the build results to the user.
func displayBuildResults(result types.BuildResult, tags []TagInfo) {
	ui.PrintSuccess("Function built successfully")
	fmt.Println()

	// Print all tags
	ui.PrintMetadata("Tags ›", "")
	for _, tag := range tags {
		fmt.Printf("  • %s/%s:%s\n", tag.Namespace, tag.Name, tag.Tag)
	}
	ui.PrintMetadata("Hash ›", result.Digest)
	fmt.Println()
	ui.PrintInfo("Build time", result.BuildTime)
}