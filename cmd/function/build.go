package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

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
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var socketPath string

func NewFunctionBuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "build [path]",
		Short:         "Build an extism function",
		Args:          cobra.MaximumNArgs(1),
		RunE:          buildFunction,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.Flags().StringVarP(&socketPath, "socket", "s", "/tmp/ignition-engine.sock", "Path to the Unix socket")
	cmd.Flags().StringArrayP("tag", "t", []string{}, "Tags for the function (can be specified multiple times)")

	return cmd
}

type TagInfo struct {
	Namespace string
	Name      string
	Tag       string
}

func buildFunction(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	log.Println(absPath)

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("path %s does not exist", absPath))
		return err
	}

	manifestPath := filepath.Join(absPath, "ignition.yml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		ui.PrintError(fmt.Sprintf("directory %s not an ignition project", absPath))
		return fmt.Errorf("not an ignition project: %s does not exist", manifestPath)
	}

	// Read and parse the manifest file
	manifestFile, err := os.ReadFile(filepath.Join(absPath, "ignition.yml"))
	if err != nil {
		return fmt.Errorf("failed to read ignition.yml: %w", err)
	}

	var functionConfig manifest.FunctionManifest
	if err := yaml.Unmarshal(manifestFile, &functionConfig); err != nil {
		return fmt.Errorf("failed to parse ignition.yml: %w", err)
	}

	// Get tags from flags
	tagInputs, _ := cmd.Flags().GetStringArray("tag")
	var tags []TagInfo

	// If no tags provided, use default namespace and directory name
	if len(tagInputs) == 0 {
		tags = append(tags, TagInfo{
			Namespace: "default",
			Name:      filepath.Base(absPath),
			Tag:       "latest",
		})
	}

	// Parse all provided tags
	for _, tagInput := range tagInputs {
		namespace, name, tag, err := parseNamespaceAndName(tagInput)
		if err != nil {
			return fmt.Errorf("invalid tag format in %q: %w", tagInput, err)
		}
		tags = append(tags, TagInfo{namespace, name, tag})
	}

	p := tea.NewProgram(spinner.NewSpinnerModelWithMessage("Building..."))

	go func() {
		buildStart := time.Now()
		var finalResult *engine.BuildResult

		// Send build requests for each tag
		for _, tagInfo := range tags {
			reqBody := engine.BuildRequest{
				Namespace: tagInfo.Namespace,
				Name:      tagInfo.Name,
				Path:      absPath,
				Manifest:  functionConfig,
				Tag:       tagInfo.Tag,
			}

			// Marshal the request body
			jsonData, err := json.Marshal(reqBody)
			if err != nil {
				p.Send(fmt.Errorf("failed to marshal request: %w", err))
				return
			}

			// Create HTTP client with Unix socket transport
			httpClient := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			}

			// Create the request
			req, err := http.NewRequest(
				"POST",
				"http://unix/build",
				bytes.NewBuffer(jsonData),
			)
			if err != nil {
				p.Send(fmt.Errorf("failed to create request: %w", err))
				return
			}
			req.Header.Set("Content-Type", "application/json")

			// Send the request
			resp, err := httpClient.Do(req)
			if err != nil {
				p.Send(fmt.Errorf("failed to send build request: %w", err))
				return
			}
			defer resp.Body.Close()

			// Check response status and parse response
			if resp.StatusCode != http.StatusOK {
				var errorMsg engine.RequestError
				if err := json.NewDecoder(resp.Body).Decode(&errorMsg); err != nil {
					p.Send(fmt.Errorf("failed to decode error response: %w", err))
					return
				}

				p.Send(errorMsg)
				return
			}

			var buildResult engine.BuildResult
			if err := json.NewDecoder(resp.Body).Decode(&buildResult); err != nil {
				p.Send(fmt.Errorf("failed to decode build response: %w", err))
				return
			}

			finalResult = &buildResult
		}

		if finalResult != nil {
			finalResult.BuildTime = time.Since(buildStart)
			p.Send(spinner.ResultMsg{Result: *finalResult})
		}
	}()

	m, err := p.Run()
	if err != nil {
		return err
	}

	finalModel := m.(spinner.SpinnerModel)
	if finalModel.HasError() {
		return finalModel.GetError()
	}

	result := finalModel.GetResult().(engine.BuildResult)
	ui.PrintSuccess("Function built successfully")
	fmt.Println()

	// Print all tags
	ui.PrintMetadata("Tags ›", "")
	for _, tag := range tags {
		fmt.Printf("  • %s/%s:%s\n", tag.Namespace, tag.Name, tag.Tag)
	}
	ui.PrintMetadata("Hash ›", result.Digest)
	fmt.Println()
	ui.PrintInfo("Build time", result.BuildTime.Round(time.Millisecond).String())
	return nil
}

func parseNamespaceAndName(input string) (namespace, name, tag string, err error) {
	// Split into namespace/name and tag
	parts := strings.Split(input, ":")
	if len(parts) > 2 {
		return "", "", "", fmt.Errorf("invalid format: expected namespace/name[:tag]")
	}

	// Extract namespace and name
	namespaceName := parts[0]
	namespaceNameParts := strings.Split(namespaceName, "/")
	if len(namespaceNameParts) != 2 {
		return "", "", "", fmt.Errorf("invalid format: expected namespace/name[:tag]")
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
		return "", "", "", fmt.Errorf("name cannot be empty")
	}

	return namespace, name, tag, nil
}
