package function

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ignitionstack/ignition/internal/ui"
	"github.com/ignitionstack/ignition/internal/ui/models/spinner"
	"github.com/ignitionstack/ignition/pkg/engine"
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

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
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

	// Create HTTP client for engine communication
	client := createEngineClient(socketPath)

	// Start the build spinner
	spinnerModel := spinner.NewSpinnerModelWithMessage("Building...")
	program := tea.NewProgram(spinnerModel)

	// Run the build in a goroutine to allow the spinner to update
	go runBuild(program, absPath, tags, functionConfig, client)

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
		return "", err
	}

	return absPath, nil
}

// loadFunctionManifest loads and parses the function manifest file.
func loadFunctionManifest(absPath string) (manifest.FunctionManifest, error) {
	var functionConfig manifest.FunctionManifest

	// Check if manifest file exists
	manifestPath := filepath.Join(absPath, "ignition.yml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("directory %s not an ignition project", absPath))
		return functionConfig, fmt.Errorf("not an ignition project: %s does not exist", manifestPath)
	}

	// Read and parse the manifest file
	manifestFile, err := os.ReadFile(manifestPath)
	if err != nil {
		return functionConfig, fmt.Errorf("failed to read ignition.yml: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(manifestFile, &functionConfig); err != nil {
		return functionConfig, fmt.Errorf("failed to parse ignition.yml: %w", err)
	}

	return functionConfig, nil
}

// parseTags parses tag information from command line flags.
func parseTags(cmd *cobra.Command, absPath string) ([]TagInfo, error) {
	var tags []TagInfo

	// Get tags from flags
	tagInputs, _ := cmd.Flags().GetStringArray("tag")

	// If no tags provided, use default namespace and directory name
	if len(tagInputs) == 0 {
		tags = append(tags, TagInfo{
			Namespace: "default",
			Name:      filepath.Base(absPath),
			Tag:       "latest",
		})
		return tags, nil
	}

	// Parse all provided tags
	for _, tagInput := range tagInputs {
		namespace, name, tag, err := parseNamespaceAndName(tagInput)
		if err != nil {
			return nil, fmt.Errorf("invalid tag format in %q: %w", tagInput, err)
		}
		tags = append(tags, TagInfo{namespace, name, tag})
	}

	return tags, nil
}

// parseNamespaceAndName parses a tag string into namespace, name, and tag components.
func parseNamespaceAndName(input string) (namespace, name, tag string, err error) {
	// Split into namespace/name and tag
	parts := strings.Split(input, ":")
	if len(parts) > 2 {
		return "", "", "", errors.New("invalid format: expected namespace/name[:tag]")
	}

	// Extract namespace and name
	namespaceName := parts[0]
	namespaceNameParts := strings.Split(namespaceName, "/")
	if len(namespaceNameParts) != 2 {
		return "", "", "", errors.New("invalid format: expected namespace/name[:tag]")
	}

	namespace = strings.TrimSpace(namespaceNameParts[0])
	name = strings.TrimSpace(namespaceNameParts[1])

	// Extract tag if provided, otherwise use "latest"
	tag = "latest"
	if len(parts) == 2 {
		tag = strings.TrimSpace(parts[1])
	}

	// If namespace or name is empty, use defaults
	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		return "", "", "", errors.New("name cannot be empty")
	}

	return namespace, name, tag, nil
}

// createEngineClient creates an HTTP client for communicating with the engine over Unix socket.
func createEngineClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}

// runBuild executes the build process and updates the spinner with progress.
func runBuild(program *tea.Program, absPath string, tags []TagInfo,
	functionConfig manifest.FunctionManifest, client *http.Client) {
	buildStart := time.Now()
	var finalResult *types.BuildResult

	// Send build requests for each tag
	for _, tagInfo := range tags {
		// Create build request
		result, err := sendBuildRequest(client, tagInfo, absPath, functionConfig)
		if err != nil {
			program.Send(err)
			return
		}
		finalResult = result
	}

	if finalResult != nil {
		finalResult.BuildTime = time.Since(buildStart).String()
		program.Send(spinner.ResultMsg{Result: *finalResult})
	}
}

// sendBuildRequest sends a single build request to the engine.
func sendBuildRequest(client *http.Client, tagInfo TagInfo,
	absPath string, functionConfig manifest.FunctionManifest) (*types.BuildResult, error) {
	// Create request body
	reqBody := engine.ExtendedBuildRequest{
		BuildRequest: types.BuildRequest{
			Namespace: tagInfo.Namespace,
			Name:      tagInfo.Name,
			Path:      absPath,
			Tag:       tagInfo.Tag,
		},
		Manifest: functionConfig,
	}

	// Marshal the request body
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the request
	req, err := http.NewRequest(
		http.MethodPost,
		"http://unix/build",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send build request: %w", err)
	}
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		var errorMsg engine.RequestError
		if err := json.NewDecoder(resp.Body).Decode(&errorMsg); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %w", err)
		}
		return nil, errorMsg
	}

	// Decode successful response
	var buildResult types.BuildResult
	if err := json.NewDecoder(resp.Body).Decode(&buildResult); err != nil {
		return nil, fmt.Errorf("failed to decode build response: %w", err)
	}

	return &buildResult, nil
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
